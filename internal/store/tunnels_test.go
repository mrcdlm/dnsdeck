package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTunnelsUpsertAndRemove(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	active := now.Add(-time.Hour)

	for _, tn := range []Tunnel{
		{ID: "a", Name: "zeta", Status: "healthy", CreatedAt: now, ConnsActiveAt: &active,
			Connections: json.RawMessage(`[{"colo_name":"fra06"}]`), LastSeenAt: now},
		{ID: "b", Name: "Alpha", Status: "down", CreatedAt: now, LastSeenAt: now},
	} {
		if err := s.UpsertTunnel(ctx, tn); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListTunnels(ctx, false)
	if err != nil || len(list) != 2 || list[0].Name != "Alpha" {
		t.Fatalf("%v %+v", err, list)
	}
	if !list[1].ConnsActiveAt.Equal(active) || string(list[1].Connections) != `[{"colo_name":"fra06"}]` {
		t.Fatalf("zeta: %+v", list[1])
	}

	// b verschwindet aus der API
	if n, err := s.MarkTunnelsRemoved(ctx, []string{"a"}, now); err != nil || n != 1 {
		t.Fatalf("removed: %v %d", err, n)
	}
	if list, _ := s.ListTunnels(ctx, false); len(list) != 1 {
		t.Fatalf("nach Entfernen: %+v", list)
	}
	if list, _ := s.ListTunnels(ctx, true); len(list) != 2 || list[0].RemovedAt == nil {
		t.Fatalf("inkl. entfernt: %+v", list)
	}
	// taucht wieder auf
	s.UpsertTunnel(ctx, Tunnel{ID: "b", Name: "Alpha", Status: "healthy", CreatedAt: now, LastSeenAt: now.Add(time.Minute)})
	if list, _ := s.ListTunnels(ctx, false); len(list) != 2 {
		t.Fatalf("wieder da: %+v", list)
	}
	// leere Liste markiert alle
	if n, _ := s.MarkTunnelsRemoved(ctx, nil, now); n != 2 {
		t.Fatalf("alle entfernt: %d", n)
	}
}

func TestTunnelSegments(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	t0 := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	gap := 3 * time.Minute
	rec := func(status string, at time.Duration) string {
		t.Helper()
		prev, err := s.RecordTunnelStatus(ctx, "a", status, t0.Add(at), gap)
		if err != nil {
			t.Fatal(err)
		}
		return prev
	}

	if p := rec("healthy", 0); p != "" {
		t.Fatalf("erste Beobachtung prev=%q", p)
	}
	rec("healthy", time.Minute)
	rec("healthy", 2*time.Minute)
	if p := rec("down", 3*time.Minute); p != "healthy" {
		t.Fatalf("Wechsel prev=%q", p)
	}
	rec("down", 4*time.Minute)
	// Lücke von 10 min (dnsdeck aus) → neuer Abschnitt trotz gleichem Status
	rec("down", 14*time.Minute)

	segs, err := s.TunnelSegments(ctx, "a", t0)
	if err != nil {
		t.Fatal(err)
	}
	want := []TunnelSegment{
		{"healthy", t0, t0.Add(2 * time.Minute)},
		{"down", t0.Add(3 * time.Minute), t0.Add(4 * time.Minute)},
		{"down", t0.Add(14 * time.Minute), t0.Add(14 * time.Minute)},
	}
	if len(segs) != len(want) {
		t.Fatalf("segs: %+v", segs)
	}
	for i := range want {
		if segs[i].Status != want[i].Status || !segs[i].StartedAt.Equal(want[i].StartedAt) || !segs[i].LastSeenAt.Equal(want[i].LastSeenAt) {
			t.Fatalf("seg %d: %+v, want %+v", i, segs[i], want[i])
		}
	}

	// since filtert Abschnitte, die vorher endeten
	if segs, _ := s.TunnelSegments(ctx, "a", t0.Add(5*time.Minute)); len(segs) != 1 {
		t.Fatalf("since: %+v", segs)
	}
	if n, _ := s.PurgeTunnelSegments(ctx, t0.Add(5*time.Minute)); n != 2 {
		t.Fatalf("purge: %d", n)
	}
}

func TestTunnelStatusChanges(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	t0 := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	s.UpsertTunnel(ctx, Tunnel{ID: "a", Name: "home", Status: "healthy", CreatedAt: t0, LastSeenAt: t0})
	gap := 3 * time.Minute
	for _, o := range []struct {
		id, status string
		at         time.Duration
	}{
		{"a", "healthy", 0}, {"a", "healthy", time.Minute}, {"a", "down", 2 * time.Minute},
		{"a", "down", 20 * time.Minute}, // Lücke, gleicher Status → kein Wechsel
		{"a", "healthy", 21 * time.Minute},
		{"b", "degraded", 5 * time.Minute}, // Tunnel ohne Snapshot → Name = ID
	} {
		if _, err := s.RecordTunnelStatus(ctx, o.id, o.status, t0.Add(o.at), gap); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.TunnelStatusChanges(ctx, TunnelChangeFilter{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, c := range all {
		got = append(got, c.TunnelName+":"+c.From+">"+c.To)
	}
	want := "home:down>healthy b:>degraded home:healthy>down home:>healthy"
	if strings.Join(got, " ") != want {
		t.Fatalf("changes = %v", got)
	}

	onlyA, _ := s.TunnelStatusChanges(ctx, TunnelChangeFilter{TunnelID: "a", Before: t0.Add(21 * time.Minute)}, 10)
	if len(onlyA) != 2 || onlyA[0].To != "down" {
		t.Fatalf("Filter: %+v", onlyA)
	}
}
