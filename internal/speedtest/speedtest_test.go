package speedtest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

// fakeCloudflare bildet __down und __up von speed.cloudflare.com nach.
func fakeCloudflare(t *testing.T, upStatus int) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var uploaded atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("GET /__down", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "" {
			t.Errorf("Kompression angefragt: %q", r.Header.Get("Accept-Encoding"))
		}
		n, _ := strconv.Atoi(r.URL.Query().Get("bytes"))
		h := w.Header()
		h.Set("colo", "FRA")
		h.Set("city", "Augsburg")
		h.Set("country", "DE")
		h.Set("cf-meta-ip", "203.0.113.7")
		h.Set("Server-Timing", "cfSpeedEdge;dur=1, cfSpeedWorker;dur=2")
		h.Add("Server-Timing", `cfL4;desc="?rtt=15000";dur=999`)
		h.Set("Content-Length", strconv.Itoa(n))
		buf := make([]byte, 32<<10)
		for n > 0 {
			k := min(n, len(buf))
			if _, err := w.Write(buf[:k]); err != nil {
				return
			}
			n -= k
		}
	})
	mux.HandleFunc("POST /__up", func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		uploaded.Add(n)
		w.WriteHeader(upStatus)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &uploaded
}

func testClient(url string) *Client {
	c := New()
	c.BaseURL, c.Streams, c.Duration, c.Warmup, c.ChunkBytes, c.Pings = url, 2, 300*time.Millisecond, 50*time.Millisecond, 256<<10, 5
	return c
}

func TestRun(t *testing.T) {
	srv, uploaded := fakeCloudflare(t, http.StatusOK)
	res, err := testClient(srv.URL).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.DownloadMbps <= 0 || res.UploadMbps <= 0 || res.DownloadBytes == 0 || res.UploadBytes == 0 {
		t.Fatalf("keine Messwerte: %+v", res)
	}
	if res.Colo != "FRA" || res.City != "Augsburg" || res.Country != "DE" || res.IP != "203.0.113.7" {
		t.Fatalf("Standort: %+v", res)
	}
	if uploaded.Load() == 0 || res.UploadBytes < uploaded.Load() {
		t.Fatalf("Upload gezählt %d, angekommen %d", res.UploadBytes, uploaded.Load())
	}
}

func TestRunErrors(t *testing.T) {
	srv, _ := fakeCloudflare(t, http.StatusForbidden)
	_, err := testClient(srv.URL).Run(context.Background())
	if m := i18n.FromError(err); m.Code != "speedtest.http" || m.Params["status"] != "403" {
		t.Fatalf("Upload-Fehler: %v", err)
	}

	_, err = testClient("http://127.0.0.1:1").Run(context.Background())
	if m := i18n.FromError(err); m.Code != "speedtest.failed" {
		t.Fatalf("nicht erreichbar: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := testClient(srv.URL).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("abgebrochen: %v", err)
	}
}

func TestMetaFromRay(t *testing.T) {
	h := http.Header{}
	h.Set("cf-ray", "a45b977fce028e0f-DUS")
	if m := metaFrom(h); m.colo != "DUS" {
		t.Fatalf("colo aus CF-RAY: %+v", m)
	}
}

func TestServerTime(t *testing.T) {
	h := http.Header{}
	h.Add("Server-Timing", "cfSpeedEdge;dur=4, cfSpeedWorker;dur=19")
	h.Add("Server-Timing", `cfL4;desc="?proto=TCP&rtt=15747";dur=500`)
	if got := serverTime(h); got != 19 {
		t.Fatalf("serverTime = %v", got)
	}
}

func TestMedianJitter(t *testing.T) {
	if m := Median([]float64{30, 10, 20}); m != 20 {
		t.Fatalf("Median ungerade: %v", m)
	}
	if m := Median([]float64{10, 20, 30, 40}); m != 25 {
		t.Fatalf("Median gerade: %v", m)
	}
	if j := Jitter([]float64{10, 14, 12}); j != 3 {
		t.Fatalf("Jitter: %v", j)
	}
	if Median(nil) != 0 || Jitter([]float64{5}) != 0 {
		t.Fatal("Randfälle")
	}
}

// --- Runner ---

type fakeMeasurer struct {
	res     Result
	err     error
	block   chan struct{} // gesetzt: wartet, bis geschlossen
	calls   atomic.Int32
	started chan struct{}
}

func (f *fakeMeasurer) Run(ctx context.Context) (Result, error) {
	f.calls.Add(1)
	if f.started != nil {
		f.started <- struct{}{}
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	return f.res, f.err
}

type memStore struct {
	mu   sync.Mutex
	list []store.Speedtest
}

func (m *memStore) InsertSpeedtest(_ context.Context, t store.Speedtest) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t.ID = int64(len(m.list) + 1)
	m.list = append(m.list, t)
	return t.ID, nil
}

func (m *memStore) LastSpeedtest(context.Context) (store.Speedtest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.list) == 0 {
		return store.Speedtest{}, store.ErrNotFound
	}
	return m.list[len(m.list)-1], nil
}

func (m *memStore) PruneSpeedtests(context.Context, time.Time) (int64, error) { return 0, nil }

func (m *memStore) all() []store.Speedtest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]store.Speedtest(nil), m.list...)
}

type topics struct {
	mu  sync.Mutex
	got []string
}

func (p *topics) Publish(t string) { p.mu.Lock(); p.got = append(p.got, t); p.mu.Unlock() }

func TestRunnerSchedule(t *testing.T) {
	ctx := context.Background()
	m := &fakeMeasurer{res: Result{DownloadMbps: 100, UploadMbps: 20, LatencyMS: 12, Colo: "FRA"}}
	st := &memStore{}
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	r := NewRunner(ctx, m, st, slog.New(slog.DiscardHandler), nil)
	r.now = func() time.Time { return now }

	// abgeschaltet: keine Messung, selten nachsehen
	if err := r.Poll(ctx); err != nil || m.calls.Load() != 0 || r.NextDelay() != idleRecheck {
		t.Fatalf("aus: calls=%d delay=%v err=%v", m.calls.Load(), r.NextDelay(), err)
	}

	r.Interval = func() time.Duration { return 6 * time.Hour }
	if err := r.Poll(ctx); err != nil || m.calls.Load() != 1 {
		t.Fatalf("fällig: calls=%d err=%v", m.calls.Load(), err)
	}
	got := st.all()[0]
	if got.Trigger != store.TriggerScheduled || *got.DownloadMbps != 100 || got.Colo != "FRA" || !got.ErrorMsg.IsZero() {
		t.Fatalf("gespeichert: %+v", got)
	}

	// nach einem Neustart zwei Stunden später: nicht fällig, Rest warten
	now = now.Add(2 * time.Hour)
	if err := r.Poll(ctx); err != nil || m.calls.Load() != 1 {
		t.Fatalf("nicht fällig: calls=%d", m.calls.Load())
	}
	if d := r.NextDelay(); d != 4*time.Hour {
		t.Fatalf("NextDelay = %v", d)
	}

	// Fehler werden mit Meldung gespeichert
	now = now.Add(5 * time.Hour)
	m.err = i18n.E("speedtest.timeout")
	if err := r.Poll(ctx); err != nil || len(st.all()) != 2 {
		t.Fatalf("Fehler: %v %d", err, len(st.all()))
	}
	if f := st.all()[1]; f.ErrorMsg.Code != "speedtest.timeout" || f.DownloadMbps != nil {
		t.Fatalf("Fehler gespeichert: %+v", f)
	}
}

func TestRunnerStartOnlyOnce(t *testing.T) {
	ctx := t.Context()
	m := &fakeMeasurer{block: make(chan struct{}), started: make(chan struct{}, 1)}
	st := &memStore{}
	pub := &topics{}
	r := NewRunner(ctx, m, st, slog.New(slog.DiscardHandler), pub)
	r.Interval = func() time.Duration { return time.Minute }

	if !r.Start(store.TriggerManual) {
		t.Fatal("Start abgelehnt")
	}
	<-m.started
	if running, _ := r.Status(); !running {
		t.Fatal("läuft nicht")
	}
	if r.Start(store.TriggerManual) {
		t.Fatal("zweite Messung parallel gestartet")
	}
	if err := r.Poll(ctx); err != nil || m.calls.Load() != 1 {
		t.Fatalf("Zeitplan während laufender Messung: calls=%d", m.calls.Load())
	}
	close(m.block)
	r.Wait()
	if running, _ := r.Status(); running || len(st.all()) != 1 || st.all()[0].Trigger != store.TriggerManual {
		t.Fatalf("nach Ende: running=%v %+v", running, st.all())
	}
	if len(pub.got) < 2 {
		t.Fatalf("Live-Updates: %v", pub.got)
	}
}

func TestRunnerShutdownDiscards(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &fakeMeasurer{block: make(chan struct{}), started: make(chan struct{}, 1)}
	st := &memStore{}
	r := NewRunner(ctx, m, st, slog.New(slog.DiscardHandler), nil)
	r.Start(store.TriggerManual)
	<-m.started
	cancel()
	r.Wait()
	if len(st.all()) != 0 {
		t.Fatalf("abgebrochene Messung gespeichert: %+v", st.all())
	}
}
