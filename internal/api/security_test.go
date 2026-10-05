package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
