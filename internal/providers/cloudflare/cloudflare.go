// Package cloudflare implementiert providers.Provider für die Cloudflare-API v4.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/providers"
)

const DefaultBaseURL = "https://api.cloudflare.com/client/v4"

// Kommentar an von dnsdeck angelegten Einträgen.
const managedComment = "managed by dnsdeck"

type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New erzeugt einen Client. baseURL leer = offizielle API (in Tests: Mock-Server).
func New(token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		token:   token,
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Name() string { return "cloudflare" }

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	Success    bool            `json:"success"`
	Errors     []apiError      `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo *struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

// APIError enthält die Fehlermeldungen der Cloudflare-API (nie das Token).
type APIError struct {
	Status int
	Errors []apiError
}

func (e *APIError) Error() string { return i18n.T(i18n.EN, e.LocalizedMsg()) }

// LocalizedMsg liefert die Meldung übersetzbar; die Texte von Cloudflare
// selbst (englisch) stehen im Parameter "detail".
func (e *APIError) LocalizedMsg() i18n.Msg {
	status := strconv.Itoa(e.Status)
	if len(e.Errors) == 0 {
		return i18n.M("cf.api_http", "status", status)
	}
	msgs := make([]string, len(e.Errors))
	for i, er := range e.Errors {
		msgs[i] = fmt.Sprintf("%s (%d)", er.Message, er.Code)
	}
	return i18n.M("cf.api_error", "status", status, "detail", strings.Join(msgs, "; "))
}

// errNotConfigured: kein Token – erkennbar per errors.Is(err, providers.ErrNotConfigured).
// networkMsg übersetzt Verbindungsfehler in eine kurze Meldung – ohne die
// angefragte URL (lang, enthält Zonen-ID und Abfrageparameter).
func networkMsg(err error) i18n.Msg {
	var netErr net.Error
	var dnsErr *net.DNSError
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return i18n.M("cf.timeout")
	case errors.As(err, &dnsErr):
		return i18n.M("cf.dns", "host", dnsErr.Name)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	return i18n.M("cf.unreachable", "detail", err.Error())
}

var errNotConfigured = &i18n.Error{Msg: i18n.M("cf.not_configured"), Wrap: providers.ErrNotConfigured}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any) (*envelope, error) {
	if c.token == "" {
		return nil, errNotConfigured
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &i18n.Error{Msg: networkMsg(err), Wrap: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &APIError{Status: resp.StatusCode}
	}
	if resp.StatusCode >= 300 || !env.Success {
		return nil, &APIError{Status: resp.StatusCode, Errors: env.Errors}
	}
	return &env, nil
}

type zoneJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListZones liefert alle Zonen, auf die das Token Zugriff hat.
func (c *Client) ListZones(ctx context.Context) ([]providers.Zone, error) {
	var out []providers.Zone
	for page := 1; ; page++ {
		q := url.Values{"per_page": {"50"}, "page": {fmt.Sprint(page)}}
		env, err := c.do(ctx, http.MethodGet, "/zones", q, nil)
		if err != nil {
			return nil, err
		}
		var zones []zoneJSON
		if err := json.Unmarshal(env.Result, &zones); err != nil {
			return nil, err
		}
		for _, z := range zones {
			out = append(out, providers.Zone{ID: z.ID, Name: z.Name})
		}
		if env.ResultInfo == nil || page >= env.ResultInfo.TotalPages || len(zones) == 0 {
			return out, nil
		}
	}
}

type recordJSON struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment,omitempty"`
}

func (j recordJSON) toRecord(zone string) providers.Record {
	return providers.Record{ID: j.ID, Zone: zone, Name: j.Name, Type: j.Type,
		Content: j.Content, TTL: j.TTL, Proxied: j.Proxied}
}

func (c *Client) GetRecord(ctx context.Context, zone, name, recordType string) (providers.Record, error) {
	q := url.Values{"type": {recordType}, "name.exact": {name}}
	env, err := c.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(zone)+"/dns_records", q, nil)
	if err != nil {
		return providers.Record{}, err
	}
	var recs []recordJSON
	if err := json.Unmarshal(env.Result, &recs); err != nil {
		return providers.Record{}, err
	}
	switch len(recs) {
	case 0:
		return providers.Record{}, providers.ErrNotFound
	case 1:
		return recs[0].toRecord(zone), nil
	default:
		// Mehrere A-/AAAA-Einträge gleichen Namens (Round-Robin) – nicht
		// raten, welcher gemeint ist.
		return providers.Record{}, i18n.E("cf.multiple_records",
			"count", strconv.Itoa(len(recs)), "type", recordType, "name", name)
	}
}

func (c *Client) UpdateRecord(ctx context.Context, r providers.Record) (providers.Record, error) {
	body := recordJSON{Name: r.Name, Type: r.Type, Content: r.Content, TTL: r.TTL, Proxied: r.Proxied}
	base := "/zones/" + url.PathEscape(r.Zone) + "/dns_records"

	var (
		env *envelope
		err error
	)
	if r.ID == "" {
		body.Comment = managedComment
		env, err = c.do(ctx, http.MethodPost, base, nil, body)
	} else {
		env, err = c.do(ctx, http.MethodPatch, base+"/"+url.PathEscape(r.ID), nil, body)
	}
	if err != nil {
		return providers.Record{}, err
	}
	var out recordJSON
	if err := json.Unmarshal(env.Result, &out); err != nil {
		return providers.Record{}, err
	}
	return out.toRecord(r.Zone), nil
}
