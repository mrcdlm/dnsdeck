package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrationsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "app.db")
	for range 2 {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
}

func TestSettings(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	if _, err := s.GetSetting(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ErrNotFound erwartet, bekam %v", err)
	}
	if err := s.SetSetting(ctx, "a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "a", "2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting(ctx, "a"); v != "2" {
		t.Fatalf("v = %q", v)
	}
}

func TestIPChanges(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	if _, err := s.LatestIPChange(ctx, "ipv4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ErrNotFound erwartet, bekam %v", err)
	}
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.InsertIPChange(ctx, IPChange{Family: "ipv4", IP: "203.0.113.1", DetectedAt: t0})
	s.InsertIPChange(ctx, IPChange{Family: "ipv6", IP: "2001:db8::1", DetectedAt: t0})
	s.InsertIPChange(ctx, IPChange{Family: "ipv4", IP: "203.0.113.2", PreviousIP: "203.0.113.1", DetectedAt: t0.Add(time.Hour)})

	c, err := s.LatestIPChange(ctx, "ipv4")
	if err != nil {
		t.Fatal(err)
	}
	if c.IP != "203.0.113.2" || c.PreviousIP != "203.0.113.1" || !c.DetectedAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("unerwartet: %+v", c)
	}

	list, err := s.ListIPChanges(ctx, IPChangeFilter{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].IP != "203.0.113.2" || list[1].Family != "ipv6" {
		t.Fatalf("unerwartet: %+v", list)
	}
	v4, _ := s.ListIPChanges(ctx, IPChangeFilter{Family: "ipv4"}, 10)
	if len(v4) != 2 {
		t.Fatalf("Filter Familie: %+v", v4)
	}
	older, _ := s.ListIPChanges(ctx, IPChangeFilter{BeforeID: list[0].ID}, 10)
	if len(older) != 2 || older[0].ID >= list[0].ID {
		t.Fatalf("Blättern: %+v", older)
	}
}

func TestSessions(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	now := time.Now()
	s.CreateSession(ctx, "aktiv", now.Add(time.Hour))
	s.CreateSession(ctx, "alt", now.Add(-time.Second))

	if ok, _ := s.SessionValid(ctx, "aktiv", now); !ok {
		t.Fatal("aktive Session ungültig")
	}
	if ok, _ := s.SessionValid(ctx, "alt", now); ok {
		t.Fatal("abgelaufene Session gültig")
	}
	if ok, _ := s.SessionValid(ctx, "gibtsnicht", now); ok {
		t.Fatal("unbekannte Session gültig")
	}
	if n, _ := s.PurgeExpiredSessions(ctx, now); n != 1 {
		t.Fatalf("purged = %d", n)
	}
	s.DeleteSession(ctx, "aktiv")
	if ok, _ := s.SessionValid(ctx, "aktiv", now); ok {
		t.Fatal("gelöschte Session gültig")
	}
}
