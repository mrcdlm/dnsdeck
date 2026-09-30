package api

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/tunnels"
)

// readEvent liest bis zur nächsten Leerzeile und liefert event- und data-Feld.
func readEvent(t *testing.T, rd *bufio.Reader) (event, data string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case line == "":
				if event != "" || data != "" {
					return
				}
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("kein Ereignis erhalten")
	}
	return event, data
}

func openStream(t *testing.T, e *testEnv, c *http.Cookie) *bufio.Reader {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/api/events", nil)
	req.AddCookie(c)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// auf Anmeldung beim Broker warten
	for i := 0; e.broker.Subscribers() == 0 && i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	return bufio.NewReader(resp.Body)
}

func TestEventsRequireAuth(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{
		{"GET", "/api/events"}, {"GET", "/api/tunnels"}, {"POST", "/api/tunnels/refresh"},
	} {
		if resp := e.do(t, p.m, p.path, "", nil); resp.StatusCode != 401 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
}

func TestEventsStream(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	rd := openStream(t, e, c)

	e.broker.Publish(events.TopicIP)
	if ev, data := readEvent(t, rd); ev != "change" || data != "ip" {
		t.Fatalf("event=%q data=%q", ev, data)
	}

	// Record anlegen → records + updates
	e.do(t, "POST", "/api/records", `{"zone_id":"z1","name":"home.example.com","type":"A"}`, c)
	seen := map[string]bool{}
	for range 2 {
		_, data := readEvent(t, rd)
		seen[data] = true
	}
	if !seen["records"] || !seen["updates"] {
		t.Fatalf("seen = %v", seen)
	}
}

func TestEventsSessionExpiry(t *testing.T) {
	old := heartbeatInterval
	heartbeatInterval = 20 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = old })

	e := newTestEnv(t, nil)
	c := e.login(t)
	rd := openStream(t, e, c)
	e.do(t, "POST", "/api/logout", "", c)

	if ev, data := readEvent(t, rd); ev != "session" || data != "expired" {
		t.Fatalf("event=%q data=%q", ev, data)
	}
}

func TestTunnelsAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	o := decode[tunnels.Overview](t, e.do(t, "GET", "/api/tunnels", "", c))
	if !o.Configured || len(o.Tunnels) != 0 || o.LastPoll != nil {
		t.Fatalf("vor Abfrage: %+v", o)
	}
	o = decode[tunnels.Overview](t, e.do(t, "POST", "/api/tunnels/refresh", "", c))
	if len(o.Tunnels) != 1 || o.Tunnels[0].Name != "home" || o.Tunnels[0].Status != "healthy" ||
		o.Tunnels[0].Uptime["24h"].Percent == nil || len(o.Tunnels[0].Uptime["7d"].Buckets) != 56 {
		t.Fatalf("nach Abfrage: %+v", o)
	}

	// Cloudflare-Fehler: 200 mit Fehlertext, letzter Stand bleibt
	e.cf.Fail(500)
	o = decode[tunnels.Overview](t, e.do(t, "POST", "/api/tunnels/refresh", "", c))
	if o.Error == "" || len(o.Tunnels) != 1 {
		t.Fatalf("bei Fehler: %+v", o)
	}
}
