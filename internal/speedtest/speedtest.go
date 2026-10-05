// Package speedtest misst Latenz, Download und Upload des Internetanschlusses
// gegen speed.cloudflare.com. Cloudflare beantwortet die Anfragen per Anycast
// aus dem nächstgelegenen Rechenzentrum – gemessen wird also immer gegen einen
// Standort in der Nähe, nicht ins Ausland.
package speedtest

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

const DefaultBaseURL = "https://speed.cloudflare.com"

// Result ist das Ergebnis einer Messung.
type Result struct {
	DownloadMbps float64
	UploadMbps   float64
	// LatencyMS: Median der Antwortzeiten im Leerlauf; JitterMS: mittlere
	// Abweichung aufeinanderfolgender Messungen.
	LatencyMS float64
	JitterMS  float64
	// Colo: Cloudflare-Rechenzentrum, gegen das gemessen wurde (IATA-Code,
	// z. B. "FRA"). City/Country: Standort des Anschlusses laut Cloudflare –
	// zeigt, dass im eigenen Land gemessen wurde.
	Colo    string
	City    string
	Country string // ISO-Code
	// IP: öffentliche Adresse, mit der gemessen wurde (zeigt IPv4/IPv6).
	IP            string
	DownloadBytes int64
	UploadBytes   int64
}

// Client führt Messungen aus. Die Felder lassen sich für Tests verkleinern.
type Client struct {
	BaseURL string
	// Streams: parallele Verbindungen je Richtung (eine einzelne TCP-
	// Verbindung lastet schnelle Anschlüsse nicht aus).
	Streams int
	// Duration: Messdauer je Richtung; die ersten Warmup werden nicht
	// gewertet (TCP-Slow-Start).
	Duration time.Duration
	Warmup   time.Duration
	// ChunkBytes: Größe einer einzelnen Down-/Upload-Anfrage.
	ChunkBytes int64
	// Pings: Anzahl der Latenzmessungen.
	Pings int
	HTTP  *http.Client
}

func New() *Client {
	return &Client{
		BaseURL: DefaultBaseURL, Streams: 6, Duration: 8 * time.Second, Warmup: 1500 * time.Millisecond,
		ChunkBytes: 25 << 20, Pings: 20, HTTP: newHTTPClient(),
	}
}

func newHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableCompression = true // Nullbytes ließen sich sonst komprimieren
	tr.MaxIdleConnsPerHost = 16
	// HTTP/1.1: eigene TCP-Verbindung je Stream (HTTP/2 bündelte alle in einer)
	tr.Protocols = new(http.Protocols)
	tr.Protocols.SetHTTP1(true)
	return &http.Client{Transport: tr}
}

// Run misst nacheinander Latenz, Download und Upload.
func (c *Client) Run(ctx context.Context) (Result, error) {
	var res Result
	lat, jit, meta, err := c.latency(ctx)
	if err != nil {
		return res, err
	}
	res.LatencyMS, res.JitterMS = lat, jit
	res.Colo, res.City, res.Country, res.IP = meta.colo, meta.city, meta.country, meta.ip

	if res.DownloadMbps, res.DownloadBytes, err = c.throughput(ctx, c.download); err != nil {
		return res, err
	}
	if res.UploadMbps, res.UploadBytes, err = c.throughput(ctx, c.upload); err != nil {
		return res, err
	}
	return res, nil
}

type serverMeta struct{ colo, city, country, ip string }

func metaFrom(h http.Header) serverMeta {
	get := func(name string) string {
		if v := h.Get(name); v != "" {
			return v
		}
		return h.Get("cf-meta-" + name)
	}
	m := serverMeta{colo: get("colo"), city: get("city"), country: get("country"), ip: h.Get("cf-meta-ip")}
	if m.colo == "" { // CF-RAY endet mit dem Rechenzentrum, z. B. "…-FRA"
		if _, colo, ok := strings.Cut(h.Get("cf-ray"), "-"); ok {
			m.colo = colo
		}
	}
	return m
}

// latency misst die Antwortzeit kleiner Anfragen auf einer bestehenden
// Verbindung. Die erste Anfrage (Verbindungsaufbau) zählt nicht; die
// Bearbeitungszeit des Servers (Server-Timing) wird abgezogen.
func (c *Client) latency(ctx context.Context) (median, jitter float64, meta serverMeta, err error) {
	var samples []float64
	for i := 0; i <= c.Pings; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/__down?bytes=0", nil)
		if err != nil {
			return 0, 0, meta, err
		}
		setUA(req)
		start := time.Now()
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return 0, 0, meta, connError(err)
		}
		elapsed := time.Since(start)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, 0, meta, i18n.E("speedtest.http", "status", strconv.Itoa(resp.StatusCode))
		}
		if i == 0 {
			meta = metaFrom(resp.Header)
			continue
		}
		ms := float64(elapsed.Microseconds())/1000 - serverTime(resp.Header)
		samples = append(samples, max(ms, 0))
	}
	return Median(samples), Jitter(samples), meta, nil
}

// serverTime liest die Bearbeitungszeit des Servers aus "Server-Timing"
// (Einträge "…;dur=<ms>"; maßgeblich ist der größte).
func serverTime(h http.Header) float64 {
	var out float64
	for _, v := range h.Values("Server-Timing") {
		for entry := range strings.SplitSeq(v, ",") {
			name, params, _ := strings.Cut(strings.TrimSpace(entry), ";")
			if name == "cfL4" { // TCP-Statistik, keine Bearbeitungszeit
				continue
			}
			for p := range strings.SplitSeq(params, ";") {
				if d, ok := strings.CutPrefix(strings.TrimSpace(p), "dur="); ok {
					if f, err := strconv.ParseFloat(d, 64); err == nil {
						out = max(out, f)
					}
				}
			}
		}
	}
	return out
}

// Median der Werte (0 bei leerer Liste).
func Median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Sorted(slices.Values(v))
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// Jitter ist die mittlere Abweichung aufeinanderfolgender Werte.
func Jitter(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	var sum float64
	for i := 1; i < len(v); i++ {
		d := v[i] - v[i-1]
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum / float64(len(v)-1)
}

// transfer überträgt in einer Schleife Daten und zählt die Bytes in n, bis ctx
// endet.
type transfer func(ctx context.Context, n *atomic.Int64) error

// throughput lässt Streams parallel übertragen und wertet die Bytes nach der
// Aufwärmphase aus.
func (c *Client) throughput(ctx context.Context, fn transfer) (mbps float64, total int64, err error) {
	runCtx, cancel := context.WithTimeout(ctx, c.Duration)
	defer cancel()

	var n atomic.Int64
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	for range max(c.Streams, 1) {
		wg.Go(func() {
			if err := fn(runCtx, &n); err != nil && runCtx.Err() == nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel() // ein Stream mit Fehler verfälscht das Ergebnis
				}
				mu.Unlock()
			}
		})
	}

	start := time.Now()
	var warmBytes int64
	warmAt := start
	select {
	case <-time.After(c.Warmup):
		warmBytes, warmAt = n.Load(), time.Now()
	case <-runCtx.Done():
	}
	wg.Wait()
	end := time.Now()

	if ctx.Err() != nil {
		return 0, n.Load(), ctx.Err()
	}
	if firstErr != nil {
		return 0, n.Load(), firstErr
	}
	bytes, secs := n.Load()-warmBytes, end.Sub(warmAt).Seconds()
	if secs <= 0 || bytes <= 0 {
		return 0, n.Load(), i18n.E("speedtest.no_data")
	}
	return float64(bytes) * 8 / secs / 1e6, n.Load(), nil
}

func (c *Client) download(ctx context.Context, n *atomic.Int64) error {
	url := c.BaseURL + "/__down?bytes=" + strconv.FormatInt(c.ChunkBytes, 10)
	buf := make([]byte, 64<<10)
	for ctx.Err() == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		setUA(req)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return connError(err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return i18n.E("speedtest.http", "status", strconv.Itoa(resp.StatusCode))
		}
		for {
			k, rerr := resp.Body.Read(buf)
			n.Add(int64(k))
			if rerr != nil {
				break
			}
		}
		resp.Body.Close()
	}
	return nil
}

func (c *Client) upload(ctx context.Context, n *atomic.Int64) error {
	for ctx.Err() == nil {
		body := &countingReader{remaining: c.ChunkBytes, n: n}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/__up", body)
		if err != nil {
			return err
		}
		req.ContentLength = c.ChunkBytes
		req.Header.Set("Content-Type", "application/octet-stream")
		setUA(req)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return connError(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return i18n.E("speedtest.http", "status", strconv.Itoa(resp.StatusCode))
		}
	}
	return nil
}

// countingReader liefert Nullbytes und zählt, was gesendet wurde.
type countingReader struct {
	remaining int64
	n         *atomic.Int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	k := min(int64(len(p)), r.remaining)
	clear(p[:k])
	r.remaining -= k
	r.n.Add(k)
	return int(k), nil
}

func setUA(req *http.Request) { req.Header.Set("User-Agent", "dnsdeck (speed test)") }

// connError übersetzt typische Verbindungsfehler in verständliche Meldungen.
func connError(err error) error {
	var dnsErr *net.DNSError
	switch {
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded):
		return i18n.E("speedtest.timeout")
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return i18n.E("speedtest.dns", "host", dnsErr.Name)
	}
	return i18n.Wrap(err, "speedtest.failed")
}
