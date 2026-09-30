package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

func envMap(m map[string]string) Env {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

type captured struct {
	method, path, query string
	header              http.Header
	body                string
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
		got = append(got, captured{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), string(b)})
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

var ev = SampleEvent(EventTunnelStatus)

func TestRenderTemplateAndPlaceholders(t *testing.T) {
	w := store.Webhook{
		Name: "Telegram", Method: "POST", ContentType: "application/json",
		URL:     "https://api.telegram.org/bot${WEBHOOK_TG_TOKEN}/sendMessage",
		Headers: []store.Header{{Name: "X-Key", Value: "Bearer ${WEBHOOK_KEY}"}},
		BodyTemplate: `{"chat_id": {{json (env "WEBHOOK_TG_CHAT")}}, "text": {{json (printf "%s\n%s" .Title .Message)}},` +
			` "prio": {{.Priority}}, "tunnel": {{json .Data.tunnel}}}`,
	}
	env := envMap(map[string]string{"WEBHOOK_TG_TOKEN": "123:abc", "WEBHOOK_KEY": "k", "WEBHOOK_TG_CHAT": "42"})
	r, err := Render(w, ev, env)
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != "https://api.telegram.org/bot123:abc/sendMessage" || r.Headers.Get("X-Key") != "Bearer k" ||
		r.Headers.Get("Content-Type") != "application/json" {
		t.Fatalf("request: %+v", r)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
		t.Fatalf("kein JSON: %s", r.Body)
	}
	if body["chat_id"] != "42" || body["text"] != "Tunnel home: getrennt\nStatus verbunden → getrennt" ||
		body["prio"] != float64(4) || body["tunnel"] != "home" {
		t.Fatalf("body: %v", body)
	}

	// Vorschau: Platzhalter bleiben, keine Geheimnisse
	p, err := Render(w, ev, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.URL+p.Body+p.Headers.Get("X-Key"), "abc") || !strings.Contains(p.URL, "${WEBHOOK_TG_TOKEN}") ||
		!strings.Contains(p.Body, `"${WEBHOOK_TG_CHAT}"`) {
		t.Fatalf("Vorschau: %+v", p)
	}

	// Leeres Template → Standard-JSON
	w.BodyTemplate = ""
	r, _ = Render(w, ev, env)
	var e Event
	if json.Unmarshal([]byte(r.Body), &e); e.Type != EventTunnelStatus || e.Title != ev.Title {
		t.Fatalf("Standard-JSON: %s", r.Body)
	}
}

func TestPlaceholderSecurity(t *testing.T) {
	env := envMap(map[string]string{"CF_API_TOKEN": "geheim", "WEBHOOK_OK": "ok"})

	// Nur WEBHOOK_-Variablen, in URL, Headern und Template
	for _, w := range []store.Webhook{
		{Method: "POST", URL: "https://evil.example/${CF_API_TOKEN}"},
		{Method: "POST", URL: "https://x.example", Headers: []store.Header{{Name: "A", Value: "${APP_PASSWORD}"}}},
		{Method: "POST", URL: "https://x.example", ContentType: "text/plain", BodyTemplate: `{{env "CF_API_TOKEN"}}`},
	} {
		if r, err := Render(w, ev, env); err == nil || strings.Contains(r.URL+r.Body, "geheim") {
			t.Errorf("Zugriff auf fremde Variable erlaubt: %+v", w)
		}
		w.Name = "x"
		if err := Validate(w); err == nil {
			t.Errorf("Validate hätte ablehnen müssen: %+v", w)
		}
	}

	// Platzhalter in Ereignisdaten werden NICHT ersetzt
	w := store.Webhook{Method: "POST", URL: "https://x.example", ContentType: "text/plain", BodyTemplate: `{{.Message}}`}
	injected := ev
	injected.Message = "Fehler: ${WEBHOOK_OK} {{env \"WEBHOOK_OK\"}}"
	r, err := Render(w, injected, env)
	if err != nil || r.Body != injected.Message {
		t.Fatalf("Ereignisdaten wurden ausgewertet: %q %v", r.Body, err)
	}

	// Fehlende Variable
	if _, err := Render(store.Webhook{Method: "POST", URL: "https://x/${WEBHOOK_FEHLT}"}, ev, env); err == nil ||
		!strings.Contains(err.Error(), "WEBHOOK_FEHLT ist nicht gesetzt") {
		t.Fatalf("fehlende Variable: %v", err)
	}
	if m := MissingEnv(store.Webhook{URL: "${WEBHOOK_OK}/${WEBHOOK_FEHLT}", BodyTemplate: `{{env "WEBHOOK_AUCH"}}`}, env); strings.Join(m, ",") != "WEBHOOK_FEHLT,WEBHOOK_AUCH" {
		t.Fatalf("MissingEnv = %v", m)
	}
}

func TestValidate(t *testing.T) {
	ok := store.Webhook{Name: "Discord", Method: "POST", URL: "${WEBHOOK_DISCORD_URL}", ContentType: "application/json",
		BodyTemplate: `{"content": {{json .Title}}}`, Events: []string{EventTunnelStatus}}
	if err := Validate(ok); err != nil {
		t.Fatalf("gültig: %v", err)
	}
	for name, mod := range map[string]func(*store.Webhook){
		"Name":        func(w *store.Webhook) { w.Name = "" },
		"Methode":     func(w *store.Webhook) { w.Method = "DELETE" },
		"URL-Schema":  func(w *store.Webhook) { w.URL = "ftp://x/${WEBHOOK_A}" },
		"Header-Name": func(w *store.Webhook) { w.Headers = []store.Header{{Name: "Bad Name", Value: "x"}} },
		"Host-Header": func(w *store.Webhook) { w.Headers = []store.Header{{Name: "Host", Value: "x"}} },
		"Ereignis":    func(w *store.Webhook) { w.Events = []string{"gibtsnicht"} },
		"Template":    func(w *store.Webhook) { w.BodyTemplate = `{{.Title` },
		"kein JSON":   func(w *store.Webhook) { w.BodyTemplate = `{"content": {{.Title}}}` },
		"Feld unbek.": func(w *store.Webhook) { w.BodyTemplate = `{{.Gibtsnicht}}` },
	} {
		w := ok
		mod(&w)
		var verr *ValidationError
		if err := Validate(w); !errors.As(err, &verr) {
			t.Errorf("%s: Validierungsfehler erwartet, bekam %v", name, err)
		}
	}
}

type memStore struct {
	mu     sync.Mutex
	hooks  []store.Webhook
	result map[int64]error
}

func (m *memStore) ListWebhooks(context.Context) ([]store.Webhook, error) { return m.hooks, nil }
func (m *memStore) GetWebhook(_ context.Context, id int64) (store.Webhook, error) {
	for _, w := range m.hooks {
		if w.ID == id {
			return w, nil
		}
	}
	return store.Webhook{}, store.ErrNotFound
}
func (m *memStore) SetWebhookResult(_ context.Context, id int64, _ time.Time, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.result[id] = err
	return nil
}
func (m *memStore) get(id int64) (error, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	err, ok := m.result[id]
	return err, ok
}

func TestDispatcher(t *testing.T) {
	ok, gotOK := recorder(t, 200)
	bad, gotBad := recorder(t, 500)
	st := &memStore{result: map[int64]error{}, hooks: []store.Webhook{
		{ID: 1, Name: "tunnel", Enabled: true, Method: "POST", URL: ok.URL + "/${WEBHOOK_PATH}", Events: []string{EventTunnelStatus}},
		{ID: 2, Name: "kaputt", Enabled: true, Method: "POST", URL: bad.URL + "/geheim", Events: []string{EventTunnelStatus}},
		{ID: 3, Name: "nur ip", Enabled: true, Method: "POST", URL: ok.URL + "/ip", Events: []string{EventIPChange}},
		{ID: 4, Name: "aus", Enabled: false, Method: "POST", URL: ok.URL + "/aus", Events: []string{EventTunnelStatus}},
	}}
	d := NewDispatcher(st, envMap(map[string]string{"WEBHOOK_PATH": "p"}), slog.New(slog.DiscardHandler))
	d.backoff = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	d.Start(ctx)
	d.Notify(ev)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, done := st.get(2); done {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	d.Wait()

	c := gotOK()
	if len(c) != 1 || c[0].path != "/p" || !strings.Contains(c[0].body, `"tunnel_status"`) {
		t.Fatalf("zugestellt: %+v (nur Webhook 1 erwartet)", c)
	}
	if len(gotBad()) != 3 {
		t.Fatalf("Wiederholungen: %d", len(gotBad()))
	}
	if err, _ := st.get(2); err == nil || err.Error() != "HTTP 500" {
		t.Fatalf("Status 2: %v", err)
	}
	if err, _ := st.get(1); err != nil {
		t.Fatalf("Status 1: %v", err)
	}

	// Test sendet auch an deaktivierte Webhooks, genau einmal
	res, err := d.SendTest(context.Background(), 4)
	if err != nil || !res.OK || gotOK()[len(gotOK())-1].path != "/aus" {
		t.Fatalf("SendTest: %+v %v", res, err)
	}
	res, _ = d.SendTest(context.Background(), 2)
	if res.OK || res.Error != "HTTP 500" || len(gotBad()) != 4 {
		t.Fatalf("SendTest Fehler: %+v, %d Versuche", res, len(gotBad()))
	}

	// Verbindungsfehler: Meldung ohne URL
	st.hooks = []store.Webhook{{ID: 9, Name: "x", Method: "POST", URL: "http://127.0.0.1:1/${WEBHOOK_PATH}?token=geheim"}}
	res, _ = d.SendTest(context.Background(), 9)
	if res.OK || strings.Contains(res.Error, "geheim") || strings.Contains(res.Error, "127.0.0.1:1/p") {
		t.Fatalf("Geheimnis in Fehlermeldung: %q", res.Error)
	}
	var nilNotifier Notifier
	Send(nilNotifier, ev)
}

func TestPlainSecretsRejected(t *testing.T) {
	base := store.Webhook{Name: "x", Method: "POST", URL: "https://x.example/hook", ContentType: "text/plain"}
	for name, mod := range map[string]func(*store.Webhook){
		"Authorization":  func(w *store.Webhook) { w.Headers = []store.Header{{Name: "Authorization", Value: "Bearer abc123"}} },
		"X-Gotify-Key":   func(w *store.Webhook) { w.Headers = []store.Header{{Name: "X-Gotify-Key", Value: "Axyz"}} },
		"Query-Token":    func(w *store.Webhook) { w.URL = "https://x.example/hook?token=abc" },
		"Userinfo":       func(w *store.Webhook) { w.URL = "https://user:pw@x.example/hook" },
		"Discord-URL":    func(w *store.Webhook) { w.URL = "https://discord.com/api/webhooks/123/abc" },
		"Slack-URL":      func(w *store.Webhook) { w.URL = "https://hooks.slack.com/services/T/B/x" },
		"Telegram-Token": func(w *store.Webhook) { w.URL = "https://api.telegram.org/bot123:abc/sendMessage" },
	} {
		w := base
		mod(&w)
		if err := Validate(w); err == nil || !strings.Contains(err.Error(), "${WEBHOOK_NAME}") {
			t.Errorf("%s: Klartext-Geheimnis nicht erkannt: %v", name, err)
		}
	}
	for name, mod := range map[string]func(*store.Webhook){
		"Header mit Platzhalter": func(w *store.Webhook) {
			w.Headers = []store.Header{{Name: "Authorization", Value: "Bearer ${WEBHOOK_T}"}}
		},
		"harmloser Header":   func(w *store.Webhook) { w.Headers = []store.Header{{Name: "X-Quelle", Value: "dnsdeck"}} },
		"Discord mit Platzh": func(w *store.Webhook) { w.URL = "${WEBHOOK_DISCORD_URL}" },
		"Telegram mit Platz": func(w *store.Webhook) { w.URL = "https://api.telegram.org/bot${WEBHOOK_TG}/sendMessage" },
		"Query ohne Geheim":  func(w *store.Webhook) { w.URL = "https://x.example/hook?format=json" },
	} {
		w := base
		mod(&w)
		if err := Validate(w); err != nil {
			t.Errorf("%s: fälschlich abgelehnt: %v", name, err)
		}
	}
}

func TestUnquotedEnvAndMul(t *testing.T) {
	w := store.Webhook{Name: "tg", Method: "POST", URL: "https://x.example", ContentType: "application/json",
		BodyTemplate: `{"chat_id": {{env "WEBHOOK_CHAT"}}, "priority": {{mul .Priority 2}}}`}
	if err := Validate(w); err != nil {
		t.Fatalf("ungequotete env-Zahl abgelehnt: %v", err)
	}
	r, err := Render(w, ev, envMap(map[string]string{"WEBHOOK_CHAT": "12345"}))
	if err != nil || r.Body != `{"chat_id": 12345, "priority": 8}` {
		t.Fatalf("body=%q err=%v", r.Body, err)
	}
	if p, err := Render(w, ev, nil); err != nil || !strings.Contains(p.Body, "${WEBHOOK_CHAT}") {
		t.Fatalf("Vorschau: %+v %v", p, err)
	}
}

func TestRedirectToOtherHostNotFollowed(t *testing.T) {
	target, gotTarget := recorder(t, 200)
	redirect := httptest.NewServer(http.RedirectHandler(target.URL+"/fremd", http.StatusFound))
	t.Cleanup(redirect.Close)
	st := &memStore{result: map[int64]error{}, hooks: []store.Webhook{{ID: 1, Name: "r", Method: "POST",
		URL: redirect.URL, Headers: []store.Header{{Name: "X-Gotify-Key", Value: "${WEBHOOK_KEY}"}}}}}
	d := NewDispatcher(st, envMap(map[string]string{"WEBHOOK_KEY": "geheim"}), slog.New(slog.DiscardHandler))
	res, _ := d.SendTest(context.Background(), 1)
	if res.OK || !strings.Contains(res.Error, "Weiterleitung") || len(gotTarget()) != 0 {
		t.Fatalf("Weiterleitung verfolgt: %+v, %d Anfragen beim Ziel", res, len(gotTarget()))
	}
}

type countPub struct{ n atomic.Int32 }

func (c *countPub) Publish(string) { c.n.Add(1) }

func TestParallelDelivery(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(700 * time.Millisecond)
		w.WriteHeader(500)
	}))
	t.Cleanup(slow.Close)
	fast, gotFast := recorder(t, 200)
	st := &memStore{result: map[int64]error{}, hooks: []store.Webhook{
		{ID: 1, Name: "langsam", Enabled: true, Method: "POST", URL: slow.URL, Events: []string{EventTunnelStatus}},
		{ID: 2, Name: "schnell", Enabled: true, Method: "POST", URL: fast.URL, Events: []string{EventTunnelStatus}},
	}}
	pub := &countPub{}
	d := NewDispatcher(st, envMap(nil), slog.New(slog.DiscardHandler))
	d.Pub, d.backoff = pub, time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	start := time.Now()
	d.Notify(ev)
	for len(gotFast()) == 0 && time.Since(start) < 3*time.Second {
		time.Sleep(5 * time.Millisecond)
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Fatalf("schneller Webhook wartete auf langsamen: %v", took)
	}
	for pub.n.Load() == 0 && time.Since(start) < 5*time.Second {
		time.Sleep(10 * time.Millisecond)
	}
	if pub.n.Load() != 1 {
		t.Fatalf("Live-Ereignis nach Zustellung fehlt: %d", pub.n.Load())
	}
}
