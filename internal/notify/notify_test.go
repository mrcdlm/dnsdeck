package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type captured struct {
	path   string
	header http.Header
	body   string
}

func recorder(t *testing.T, status int) (*httptest.Server, func() []captured) {
	t.Helper()
	var (
		mu  sync.Mutex
		got []captured
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, captured{r.URL.Path, r.Header.Clone(), string(b)})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []captured {
		mu.Lock()
		defer mu.Unlock()
		return append([]captured{}, got...)
	}
}

var ev = Event{Type: EventTunnelStatus, Title: "Tunnel home getrennt", Message: "healthy → down", Priority: PriorityHigh}

func TestChannels(t *testing.T) {
	srv, got := recorder(t, 200)
	ctx := context.Background()

	if err := (Webhook{URL: srv.URL + "/hook"}).Send(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := (Ntfy{URL: srv.URL + "/geheimes-topic", Token: "tk"}).Send(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := (Gotify{URL: srv.URL + "/", Token: "app"}).Send(ctx, ev); err != nil {
		t.Fatal(err)
	}
	c := got()
	if len(c) != 3 {
		t.Fatalf("%d Anfragen", len(c))
	}
	var hook Event
	json.Unmarshal([]byte(c[0].body), &hook)
	if c[0].path != "/hook" || hook.Title != ev.Title || hook.Type != EventTunnelStatus {
		t.Fatalf("webhook: %+v", c[0])
	}
	if c[1].path != "/geheimes-topic" || c[1].body != ev.Message || c[1].header.Get("Title") != ev.Title ||
		c[1].header.Get("Priority") != "4" || c[1].header.Get("Authorization") != "Bearer tk" || c[1].header.Get("Tags") == "" {
		t.Fatalf("ntfy: %+v", c[1])
	}
	if c[2].path != "/message" || c[2].header.Get("X-Gotify-Key") != "app" || !strings.Contains(c[2].body, `"priority":8`) {
		t.Fatalf("gotify: %+v", c[2])
	}
}

func TestErrorsHideSecrets(t *testing.T) {
	srv, _ := recorder(t, 403)
	err := (Ntfy{URL: srv.URL + "/geheimes-topic"}).Send(context.Background(), ev)
	if err == nil || err.Error() != "HTTP 403" {
		t.Fatalf("err = %v", err)
	}
	err = (Webhook{URL: "http://127.0.0.1:1/geheim?token=abc"}).Send(context.Background(), ev)
	if err == nil || strings.Contains(err.Error(), "geheim") || strings.Contains(err.Error(), "abc") {
		t.Fatalf("Geheimnis in Fehlermeldung: %v", err)
	}
	if got := (Webhook{URL: "https://user:pw@hooks.example.com/abc?x=1"}).Target(); got != "https://hooks.example.com/…" {
		t.Fatalf("Target = %q", got)
	}
}

func TestChannelsFromEnv(t *testing.T) {
	env := map[string]string{
		"NOTIFY_WEBHOOK_URL": "https://example.com/hook",
		"NOTIFY_NTFY_URL":    "https://ntfy.sh/topic",
		"NOTIFY_GOTIFY_URL":  "https://gotify.example.com",
	}
	chs, err := ChannelsFromEnv(func(k string) string { return env[k] })
	if len(chs) != 2 || err == nil || !strings.Contains(err.Error(), "NOTIFY_GOTIFY_TOKEN") {
		t.Fatalf("chs=%d err=%v", len(chs), err)
	}
	env["NOTIFY_GOTIFY_TOKEN"] = "x"
	env["NOTIFY_WEBHOOK_URL"] = "ftp://nope"
	chs, err = ChannelsFromEnv(func(k string) string { return env[k] })
	if len(chs) != 2 || err == nil {
		t.Fatalf("chs=%d err=%v", len(chs), err)
	}
	if chs, err := ChannelsFromEnv(func(string) string { return "" }); len(chs) != 0 || err != nil {
		t.Fatalf("leer: %v %v", chs, err)
	}
}

func TestDispatcher(t *testing.T) {
	ok, gotOK := recorder(t, 200)
	bad, gotBad := recorder(t, 500)
	enabled := map[string]bool{EventTunnelStatus: true}
	d := NewDispatcher([]Channel{Webhook{URL: ok.URL}, Webhook{URL: bad.URL}},
		func(t string) bool { return enabled[t] }, slog.New(slog.DiscardHandler))
	d.backoff = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	d.Start(ctx)

	d.Notify(Event{Type: EventIPChange, Title: "aus"}) // deaktiviert → nicht gesendet
	d.Notify(ev)

	deadline := time.Now().Add(3 * time.Second)
	for (len(gotOK()) < 1 || len(gotBad()) < 3) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	d.Wait()

	if len(gotOK()) != 1 || len(gotBad()) != 3 {
		t.Fatalf("ok=%d bad=%d (3 Versuche erwartet)", len(gotOK()), len(gotBad()))
	}
	st := d.Status()
	if st[0].LastSent == nil || st[0].LastErr != "" || st[1].LastErr != "HTTP 500" {
		t.Fatalf("status: %+v", st)
	}

	res := d.SendTest(context.Background())
	if !res[0].OK || res[1].OK || res[1].Error != "HTTP 500" {
		t.Fatalf("test: %+v", res)
	}
	var nilNotifier Notifier
	Send(nilNotifier, ev) // nil ist erlaubt
}
