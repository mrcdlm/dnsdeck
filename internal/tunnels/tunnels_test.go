package tunnels

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare"
	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare/cftest"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

var t0 = time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

func seg(status string, from, to time.Duration) store.TunnelSegment {
	return store.TunnelSegment{Status: status, StartedAt: t0.Add(from), LastSeenAt: t0.Add(to)}
}

func TestComputeUptime(t *testing.T) {
	h := time.Hour
	// 4 h Zeitraum, 4 Stücke à 1 h; Beobachtung gilt 1 min über last_seen hinaus
	segs := []store.TunnelSegment{
		seg(StatusHealthy, 0, 59*time.Minute),                 // Stück 0: healthy
		seg(StatusDown, h, h+29*time.Minute),                  // Stück 1: 30 min down …
		seg(StatusHealthy, h+30*time.Minute, 2*h-time.Minute), // … dann healthy → down (schlechtester)
		seg(StatusDegraded, 3*h, 4*h-time.Minute),             // Stück 2: Lücke; Stück 3: degraded
	}
	u := ComputeUptime(segs, t0, t0.Add(4*h), 4, time.Minute)

	want := []string{StatusHealthy, StatusDown, "", StatusDegraded}
	for i := range want {
		if u.Buckets[i] != want[i] {
			t.Fatalf("Buckets = %v, want %v", u.Buckets, want)
		}
	}
	// beobachtet: 3 h, davon 30 min down → 2,5/3 = 83,33 %
	if u.Percent == nil || math.Abs(*u.Percent-83.333) > 0.01 {
		t.Fatalf("Percent = %v", u.Percent)
	}
	if math.Abs(u.Observed-0.75) > 0.001 || u.BucketSeconds != 3600 {
		t.Fatalf("Observed = %v, BucketSeconds = %d", u.Observed, u.BucketSeconds)
	}

	// Nichts beobachtet → Percent nil
	if u := ComputeUptime(nil, t0, t0.Add(h), 2, time.Minute); u.Percent != nil || u.Buckets[0] != "" {
		t.Fatalf("leer: %+v", u)
	}
	// Abschnitt vor dem Zeitraum wird abgeschnitten
	u = ComputeUptime([]store.TunnelSegment{seg(StatusDown, -2*h, h)}, t0, t0.Add(2*h), 2, 0)
	if u.Buckets[0] != StatusDown || u.Buckets[1] != "" || *u.Percent != 0 {
		t.Fatalf("abgeschnitten: %+v", u)
	}
}

func TestStatusSince(t *testing.T) {
	segs := []store.TunnelSegment{
		seg(StatusDown, 0, 10*time.Minute),
		seg(StatusHealthy, 11*time.Minute, 20*time.Minute),
		seg(StatusHealthy, 22*time.Minute, 30*time.Minute), // kleine Lücke → zusammenhängend
	}
	if s := StatusSince(segs, StatusHealthy, 3*time.Minute); s == nil || !s.Equal(t0.Add(11*time.Minute)) {
		t.Fatalf("since = %v", s)
	}
	if s := StatusSince(segs, StatusHealthy, time.Minute); !s.Equal(t0.Add(22 * time.Minute)) {
		t.Fatalf("mit Lücke: %v", s)
	}
	if s := StatusSince(segs, StatusDown, time.Minute); s != nil {
		t.Fatalf("anderer Status: %v", s)
	}
}

type countPub struct{ n int }

func (c *countPub) Publish(topic string) {
	if topic == events.TopicTunnels {
		c.n++
	}
}

func TestMonitor(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	fake := cftest.New("tok")
	fake.SetAccount("acc")
	fake.AddTunnel("t1", "home", StatusHealthy)
	fake.AddTunnel("t2", "nas", StatusHealthy)
	srv := httptest.NewServer(fake)
	defer srv.Close()

	pub := &countPub{}
	m := NewMonitor(cloudflare.New("tok", srv.URL), "acc", st, slog.New(slog.DiscardHandler), pub, nil)
	clock := t0
	m.now = func() time.Time { return clock }
	var changes []Change
	m.OnChange = func(c Change) { changes = append(changes, c) }

	poll := func() {
		t.Helper()
		if err := m.Poll(ctx); err != nil {
			t.Fatal(err)
		}
		clock = clock.Add(time.Minute)
	}
	for range 10 {
		poll()
	}
	fake.SetTunnelStatus("t1", StatusDown)
	for range 5 {
		poll()
	}
	fake.SetTunnelStatus("t1", StatusHealthy)
	fake.RemoveTunnel("t2")
	poll()

	if len(changes) != 2 || changes[0].To != StatusDown || changes[1].From != StatusDown || changes[0].Name != "home" {
		t.Fatalf("changes: %+v", changes)
	}
	if pub.n != 16 {
		t.Fatalf("publish = %d", pub.n)
	}

	o, err := m.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !o.Configured || o.Error != "" || o.LastPoll == nil || len(o.Tunnels) != 1 {
		t.Fatalf("overview: %+v", o)
	}
	home := o.Tunnels[0]
	if home.Name != "home" || home.Status != StatusHealthy || !home.StatusSince.Equal(t0.Add(15*time.Minute)) {
		t.Fatalf("home: %+v since=%v", home.Tunnel, home.StatusSince)
	}
	if !strings.Contains(string(home.Connections), "fra06") {
		t.Fatalf("connections: %s", home.Connections)
	}
	// 16 Beobachtungen à 1 min: 5 min down → 11/16 up
	p := home.Uptime["24h"].Percent
	if p == nil || math.Abs(*p-11.0/16*100) > 0.01 {
		t.Fatalf("uptime 24h = %v", p)
	}

	// Cloudflare-Fehler: Stand bleibt, Fehler sichtbar, keine neuen Abschnitte
	fake.Fail(500)
	if err := m.Poll(ctx); err == nil {
		t.Fatal("Fehler erwartet")
	}
	if o, _ := m.Overview(ctx); o.Error == "" || len(o.Tunnels) != 1 {
		t.Fatalf("nach Fehler: %+v", o)
	}
}

func TestMonitorNotConfigured(t *testing.T) {
	m := NewMonitor(&fakeLister{}, "", nil, slog.New(slog.DiscardHandler), nil, nil)
	if m.Configured() {
		t.Fatal("ohne Account-ID nicht konfiguriert")
	}
	if err := m.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type fakeLister struct{}

func (fakeLister) ListTunnels(context.Context, string) ([]cloudflare.Tunnel, error) {
	return nil, errors.New("nicht aufrufen")
}
