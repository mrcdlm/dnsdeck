package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mrcdlm/dnsdeck/internal/ddns"
	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/ipdetect"
	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare"
	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare/cftest"
	"github.com/mrcdlm/dnsdeck/internal/store"
	"github.com/mrcdlm/dnsdeck/internal/tunnels"
)

type fakeTracker struct{}

func (fakeTracker) State() ipdetect.State {
	return ipdetect.State{IPv4: ipdetect.FamilyState{IP: "203.0.113.1", Status: ipdetect.StatusOK}}
}

// fakeDDNS nutzt den echten Updater mit fester IP gegen die Cloudflare-Nachbildung.
type fakeDDNS struct {
	u      *ddns.Updater
	checks int
}

func (f *fakeDDNS) RunCycle(ctx context.Context, trigger string) (ipdetect.State, error) {
	f.checks++
	st := fakeTracker{}.State()
	return st, f.u.SyncAll(ctx, ddns.IPsFromState(st), trigger)
}

func (f *fakeDDNS) SyncRecord(ctx context.Context, id int64, trigger string) (store.Record, error) {
	return f.u.SyncRecord(ctx, id, ddns.IPsFromState(fakeTracker{}.State()), trigger)
}

type testEnv struct {
	srv     *httptest.Server
	tracker *fakeDDNS
	auth    *Auth
	cf      *cftest.Fake
	broker  *events.Broker
}

func newTestEnv(t *testing.T, static fstest.MapFS) *testEnv {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	auth, err := NewAuth("richtig", st)
	if err != nil {
		t.Fatal(err)
	}
	auth.delay = 0
	if static == nil {
		static = fstest.MapFS{}
	}
	log := slog.New(slog.DiscardHandler)
	fake := cftest.New("tok", cftest.Zone{ID: "z1", Name: "example.com"})
	fake.SetAccount("acc")
	fake.AddTunnel("t1", "home", "healthy")
	cfSrv := httptest.NewServer(fake)
	t.Cleanup(cfSrv.Close)
	cf := cloudflare.New("tok", cfSrv.URL)
	broker := events.NewBroker()
	t.Cleanup(broker.Close)
	dd := &fakeDDNS{u: ddns.NewUpdater(st, cf, log)}
	dd.u.Pub = broker
	mon := tunnels.NewMonitor(cf, "acc", st, log, broker, nil)

	s := NewServer(Deps{Store: st, Tracker: fakeTracker{}, DDNS: dd, Zones: cf,
		Tunnels: mon, Events: broker, Auth: auth, Log: log, Static: static})
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, tracker: dd, auth: auth, cf: fake, broker: broker}
}

func (e *testEnv) do(t *testing.T, method, path, body string, cookie *http.Cookie) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (e *testEnv) login(t *testing.T) *http.Cookie {
	t.Helper()
	resp := e.do(t, "POST", "/api/login", `{"password":"richtig"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatal("kein Session-Cookie")
	return nil
}

func TestHealthz(t *testing.T) {
	e := newTestEnv(t, nil)
	resp := e.do(t, "GET", "/healthz", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAuthRequired(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{
		{"GET", "/api/ip"}, {"GET", "/api/ip/history"}, {"POST", "/api/ip/refresh"},
	} {
		if resp := e.do(t, p.m, p.path, "", nil); resp.StatusCode != 401 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
	bogus := &http.Cookie{Name: sessionCookie, Value: "erfunden"}
	if resp := e.do(t, "GET", "/api/ip", "", bogus); resp.StatusCode != 401 {
		t.Errorf("erfundenes Token akzeptiert: %d", resp.StatusCode)
	}
}

func TestLoginFlow(t *testing.T) {
	e := newTestEnv(t, nil)

	if resp := e.do(t, "POST", "/api/login", `{"password":"falsch"}`, nil); resp.StatusCode != 401 {
		t.Fatalf("falsches Passwort: %d", resp.StatusCode)
	}

	c := e.login(t)
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("Cookie-Attribute: %+v", c)
	}

	resp := e.do(t, "GET", "/api/ip", "", c)
	if resp.StatusCode != 200 {
		t.Fatalf("/api/ip: %d", resp.StatusCode)
	}
	var st ipdetect.State
	json.NewDecoder(resp.Body).Decode(&st)
	if st.IPv4.IP != "203.0.113.1" {
		t.Fatalf("state: %+v", st)
	}

	if resp := e.do(t, "POST", "/api/ip/refresh", "", c); resp.StatusCode != 200 || e.tracker.checks != 1 {
		t.Fatalf("refresh: %d, checks=%d", resp.StatusCode, e.tracker.checks)
	}

	resp = e.do(t, "GET", "/api/session", "", c)
	var sess map[string]bool
	json.NewDecoder(resp.Body).Decode(&sess)
	if !sess["authenticated"] {
		t.Fatal("session nicht authentifiziert")
	}

	e.do(t, "POST", "/api/logout", "", c)
	if resp := e.do(t, "GET", "/api/ip", "", c); resp.StatusCode != 401 {
		t.Fatalf("nach Logout: %d", resp.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newTestEnv(t, nil)
	for range maxFailures {
		e.do(t, "POST", "/api/login", `{"password":"falsch"}`, nil)
	}
	if resp := e.do(t, "POST", "/api/login", `{"password":"richtig"}`, nil); resp.StatusCode != 429 {
		t.Fatalf("429 erwartet, bekam %d", resp.StatusCode)
	}
	// nach Ablauf des Fensters wieder erlaubt
	e.auth.now = func() time.Time { return time.Now().Add(2 * failureWindow) }
	if resp := e.do(t, "POST", "/api/login", `{"password":"richtig"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("nach Fenster: %d", resp.StatusCode)
	}
}

func TestNewAuthAcceptsBcryptHash(t *testing.T) {
	h, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	a, err := NewAuth(string(h), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !a.checkPassword("pw") || a.checkPassword(string(h)) {
		t.Fatal("Hash-Modus funktioniert nicht")
	}
}

func TestSecureCookieBehindProxy(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/login", nil)
	if sessionCookieFor(r, "x", time.Now()).Secure {
		t.Fatal("Secure ohne HTTPS")
	}
	r.Header.Set("X-Forwarded-Proto", "https")
	if !sessionCookieFor(r, "x", time.Now()).Secure {
		t.Fatal("Secure hinter HTTPS-Proxy fehlt")
	}
}

func TestSPA(t *testing.T) {
	e := newTestEnv(t, fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	})

	resp := e.do(t, "GET", "/assets/app.js", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}

	resp = e.do(t, "GET", "/verlauf", "", nil)
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if resp.StatusCode != 200 || buf.String() != "<html>app</html>" {
		t.Fatalf("SPA-Fallback: %d %q", resp.StatusCode, buf.String())
	}

	if resp := e.do(t, "GET", "/api/gibtsnicht", "", nil); resp.StatusCode != 404 {
		t.Fatalf("API-404 erwartet: %d", resp.StatusCode)
	}
}

func TestNotBuiltPage(t *testing.T) {
	e := newTestEnv(t, nil)
	resp := e.do(t, "GET", "/", "", nil)
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(buf.String(), "nicht gebaut") {
		t.Fatalf("%d %q", resp.StatusCode, buf.String())
	}
}
