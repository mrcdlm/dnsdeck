package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestClientIP(t *testing.T) {
	s := &Server{trustedProxies: []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("::1/128")}}
	cases := []struct {
		name, remote string
		hdr          map[string]string
		want         string
	}{
		{"direkt", "203.0.113.9:5000", nil, "203.0.113.9"},
		{"gefälschter Header ohne Proxy", "203.0.113.9:5000",
			map[string]string{"CF-Connecting-IP": "1.2.3.4", "X-Forwarded-For": "1.2.3.4"}, "203.0.113.9"},
		{"Cloudflare Tunnel", "172.18.0.5:4000", map[string]string{"CF-Connecting-IP": "198.51.100.7"}, "198.51.100.7"},
		{"Reverse Proxy", "172.18.0.5:4000", map[string]string{"X-Forwarded-For": "198.51.100.7"}, "198.51.100.7"},
		// Der Client trägt selbst "1.2.3.4" ein; maßgeblich ist, was der Proxy angehängt hat.
		{"XFF links gefälscht", "172.18.0.5:4000",
			map[string]string{"X-Forwarded-For": "1.2.3.4, 198.51.100.7, 172.18.0.9"}, "198.51.100.7"},
		{"Proxy ohne Header", "172.18.0.5:4000", nil, "172.18.0.5"},
		{"IPv6-Proxy", "[::1]:4000", map[string]string{"X-Forwarded-For": "2001:db8::7"}, "2001:db8::7"},
		{"Unsinn im Header", "172.18.0.5:4000", map[string]string{"X-Forwarded-For": "kein-ip"}, "172.18.0.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/login", nil)
			r.RemoteAddr = c.remote
			for k, v := range c.hdr {
				r.Header.Set(k, v)
			}
			if got := s.clientIP(r); got != c.want {
				t.Errorf("%s, erwartet %s", got, c.want)
			}
		})
	}
}

// Hinter einem Proxy sperrt ein Angreifer mit Fehlversuchen nicht alle anderen aus.
func TestLoginRateLimitPerClientBehindProxy(t *testing.T) {
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
	for range maxFailures {
		login("198.51.100.66", "falsch")
	}
	if code := login("198.51.100.66", "falsch"); code != 429 {
		t.Fatalf("Angreifer: %d statt 429", code)
	}
	if code := login("198.51.100.7", "richtig"); code != 200 {
		t.Fatalf("anderer Client: %d statt 200", code)
	}
}
