package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// maskURL zeigt nur Schema und Host – Pfad, Query und Userinfo können
// Geheimnisse enthalten (Webhook-Token, ntfy-Topic).
func maskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(ungültige URL)"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

// post sendet eine Anfrage und liefert bei Fehlern eine Meldung ohne URL
// (die URL kann Geheimnisse enthalten).
func post(ctx context.Context, rawURL string, body []byte, header http.Header) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return errors.New("ungültige URL")
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", "dnsdeck")
	resp, err := httpClient.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err // ohne URL
		}
		return fmt.Errorf("nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// Webhook sendet das Ereignis als JSON per POST.
type Webhook struct{ URL string }

func (w Webhook) Type() string   { return "webhook" }
func (w Webhook) Target() string { return maskURL(w.URL) }
func (w Webhook) Send(ctx context.Context, ev Event) error {
	body, _ := json.Marshal(ev)
	return post(ctx, w.URL, body, http.Header{"Content-Type": {"application/json"}})
}

// Ntfy sendet an ein ntfy-Topic (URL = Server + Topic, z. B. https://ntfy.sh/mein-topic).
type Ntfy struct {
	URL   string
	Token string // optional: Access-Token
}

var ntfyTags = map[string]string{
	EventIPChange: "globe_with_meridians", EventUpdateFailed: "warning",
	EventUpdateRecovered: "white_check_mark", EventTunnelStatus: "electric_plug", EventTest: "test_tube",
}

func (n Ntfy) Type() string   { return "ntfy" }
func (n Ntfy) Target() string { return maskURL(n.URL) }
func (n Ntfy) Send(ctx context.Context, ev Event) error {
	h := http.Header{
		"Title":        {ev.Title},
		"Priority":     {strconv.Itoa(ev.Priority)},
		"Content-Type": {"text/plain; charset=utf-8"},
	}
	if tag := ntfyTags[ev.Type]; tag != "" {
		h.Set("Tags", tag)
	}
	if n.Token != "" {
		h.Set("Authorization", "Bearer "+n.Token)
	}
	return post(ctx, n.URL, []byte(ev.Message), h)
}

// Gotify sendet über die Gotify-API (URL = Server, Token = App-Token).
type Gotify struct {
	URL   string
	Token string
}

func (g Gotify) Type() string   { return "gotify" }
func (g Gotify) Target() string { return maskURL(g.URL) }
func (g Gotify) Send(ctx context.Context, ev Event) error {
	// Gotify-Prioritäten 0–10; ntfy-ähnliche Skala grob abbilden
	body, _ := json.Marshal(map[string]any{
		"title": ev.Title, "message": ev.Message, "priority": ev.Priority * 2,
	})
	return post(ctx, strings.TrimRight(g.URL, "/")+"/message", body, http.Header{
		"Content-Type": {"application/json"},
		"X-Gotify-Key": {g.Token},
	})
}

// ChannelsFromEnv baut die Kanäle aus Env-Variablen (leere werden übersprungen).
func ChannelsFromEnv(getenv func(string) string) ([]Channel, error) {
	var out []Channel
	var errs []error
	check := func(name, raw string) bool {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s ist keine gültige http(s)-URL", name))
			return false
		}
		return true
	}
	if v := getenv("NOTIFY_WEBHOOK_URL"); v != "" && check("NOTIFY_WEBHOOK_URL", v) {
		out = append(out, Webhook{URL: v})
	}
	if v := getenv("NOTIFY_NTFY_URL"); v != "" && check("NOTIFY_NTFY_URL", v) {
		out = append(out, Ntfy{URL: v, Token: getenv("NOTIFY_NTFY_TOKEN")})
	}
	if v := getenv("NOTIFY_GOTIFY_URL"); v != "" && check("NOTIFY_GOTIFY_URL", v) {
		if tok := getenv("NOTIFY_GOTIFY_TOKEN"); tok == "" {
			errs = append(errs, errors.New("NOTIFY_GOTIFY_TOKEN fehlt"))
		} else {
			out = append(out, Gotify{URL: v, Token: tok})
		}
	}
	return out, errors.Join(errs...)
}
