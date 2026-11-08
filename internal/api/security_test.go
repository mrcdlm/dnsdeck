package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

// Parallele Anmeldeversuche dürfen die Grenze nicht aushebeln: gezählt wird
// vor der (langsamen) Passwortprüfung.
func TestLoginRateLimitParallel(t *testing.T) {
	e := newTestEnv(t, nil)
	const n = 3 * maxFailures
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			codes[i] = e.do(t, "POST", "/api/login", `{"password":"falsch"}`, nil).StatusCode
		})
	}
	wg.Wait()
	var checked int
	for _, c := range codes {
		if c == http.StatusUnauthorized {
			checked++
		}
	}
	if checked > maxFailures {
		t.Fatalf("%d Passwörter geprüft, erlaubt sind %d (%v)", checked, maxFailures, codes)
	}
}

func TestLoginSuccessResetsAttempts(t *testing.T) {
	e := newTestEnv(t, nil)
	for range maxFailures - 1 {
		e.do(t, "POST", "/api/login", `{"password":"falsch"}`, nil)
	}
	e.login(t)
	for range maxFailures - 1 {
		if resp := e.do(t, "POST", "/api/login", `{"password":"falsch"}`, nil); resp.StatusCode != 401 {
			t.Fatalf("nach erfolgreichem Login: %d", resp.StatusCode)
		}
	}
}

// Ein neues APP_PASSWORD meldet alle bestehenden Sessions ab.
func TestSessionsBoundToPassword(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	old, _ := NewAuth("altes-passwort", st)
	token, _, err := old.newSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/ip", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})

	if ok, _ := old.valid(req); !ok {
		t.Fatal("Session mit gleichem Passwort ungültig")
	}
	again, _ := NewAuth("altes-passwort", st) // Neustart, gleiches Passwort
	if ok, _ := again.valid(req); !ok {
		t.Fatal("Session nach Neustart ungültig")
	}
	changed, _ := NewAuth("neues-passwort", st)
	if ok, _ := changed.valid(req); ok {
		t.Fatal("Session nach Passwortwechsel noch gültig")
	}
}

// CSRF: Schreibende Anfragen von anderen Origins – auch von Nachbar-
// Subdomains (gleiche Site) – werden abgelehnt, auch mit gültiger Session.
func TestCrossOriginRequestsRejected(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	body := `{"name":"x","enabled":true,"method":"POST","url":"https://evil.example/","headers":[],` +
		`"content_type":"application/json","events":["ip_change"],"body_template":""}`

	send := func(method, path string, hdr map[string]string) int {
		req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "text/plain") // wie ein HTML-Formular
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		req.AddCookie(c)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	for _, hdr := range []map[string]string{
		{"Sec-Fetch-Site": "same-site"},
		{"Sec-Fetch-Site": "cross-site"},
		{"Origin": "https://evil.example"},
	} {
		if code := send("POST", "/api/webhooks", hdr); code != http.StatusForbidden {
			t.Errorf("%v: %d statt 403", hdr, code)
		}
		if code := send("POST", "/api/login", hdr); code != http.StatusForbidden {
			t.Errorf("Login %v: %d statt 403", hdr, code)
		}
	}
	// Lesende Anfragen und eigene Origin bleiben erlaubt
	if code := send("GET", "/api/webhooks", map[string]string{"Sec-Fetch-Site": "cross-site"}); code != 200 {
		t.Errorf("GET: %d", code)
	}
	if code := send("POST", "/api/webhooks", map[string]string{"Sec-Fetch-Site": "same-origin"}); code != http.StatusCreated {
		t.Errorf("eigene Origin: %d", code)
	}
}

func TestLimitKey(t *testing.T) {
	for in, want := range map[string]string{
		"198.51.100.7":          "198.51.100.7",
		"2001:db8:1:2:aaaa::1":  "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::99": "2001:db8:1:2::/64",
		"fe80::1%eth0":          "fe80::/64",
		"::ffff:198.51.100.7":   "::ffff:198.51.100.7",
		"kein-ip":               "kein-ip",
	} {
		if got := limitKey(in); got != want {
			t.Errorf("limitKey(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

// Wer per IPv6 für jeden Versuch eine neue Adresse aus seinem /64 nimmt,
// umgeht die Login-Sperre nicht; ein anderes Netz bleibt davon unberührt.
func TestLoginRateLimitIPv6Subnet(t *testing.T) {
	e := newTestEnv(t, nil)
	e.api.trustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	login := func(ip, pw string) int {
		r, _ := http.NewRequest("POST", e.srv.URL+"/api/login", strings.NewReader(`{"password":"`+pw+`"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("CF-Connecting-IP", ip)
		resp, err := e.srv.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := range maxFailures {
		login(fmt.Sprintf("2001:db8:1:2::%x", i+1), "falsch")
	}
	if code := login("2001:db8:1:2:dead:beef::1", "richtig"); code != 429 {
		t.Fatalf("neue Adresse im selben /64: %d statt 429", code)
	}
	if code := login("2001:db8:1:3::1", "richtig"); code != 200 {
		t.Fatalf("anderes /64: %d statt 200", code)
	}
}

// Sind alle Plätze für Passwortprüfungen belegt, wird ein weiterer Versuch
// nach kurzer Wartezeit abgewiesen, statt zusätzlich CPU zu belegen.
func TestLoginConcurrentChecksLimited(t *testing.T) {
	e := newTestEnv(t, nil)
	e.api.auth.checkWait = 50 * time.Millisecond
	for range maxConcurrentChecks {
		e.api.auth.checks <- struct{}{}
	}
	if code := e.do(t, "POST", "/api/login", `{"password":"richtig"}`, nil).StatusCode; code != 429 {
		t.Fatalf("alle Plätze belegt: %d statt 429", code)
	}
	e.api.auth.releaseCheck()
	if code := e.do(t, "POST", "/api/login", `{"password":"richtig"}`, nil).StatusCode; code != 200 {
		t.Fatalf("Platz frei: %d statt 200", code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := newTestEnv(t, nil)
	resp := e.do(t, "GET", "/healthz", "", nil)
	for _, h := range []string{"Content-Security-Policy", "X-Frame-Options", "X-Content-Type-Options",
		"Cross-Origin-Opener-Policy", "Cross-Origin-Resource-Policy", "Permissions-Policy"} {
		if resp.Header.Get(h) == "" {
			t.Errorf("Header %s fehlt", h)
		}
	}
}
