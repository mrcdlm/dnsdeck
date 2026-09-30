package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mrcdlm/dnsdeck/internal/providers"
	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare/cftest"
)

func setup(t *testing.T) (*Client, *cftest.Fake) {
	t.Helper()
	fake := cftest.New("test-token", cftest.Zone{ID: "z1", Name: "example.com"})
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return New("test-token", srv.URL), fake
}

func TestListZones(t *testing.T) {
	c, _ := setup(t)
	zones, err := c.ListZones(context.Background())
	if err != nil || len(zones) != 1 || zones[0].Name != "example.com" {
		t.Fatalf("%v %+v", err, zones)
	}
}

func TestGetCreateUpdate(t *testing.T) {
	c, fake := setup(t)
	ctx := context.Background()

	if _, err := c.GetRecord(ctx, "z1", "home.example.com", "A"); !errors.Is(err, providers.ErrNotFound) {
		t.Fatalf("ErrNotFound erwartet, bekam %v", err)
	}

	created, err := c.UpdateRecord(ctx, providers.Record{Zone: "z1", Name: "home.example.com",
		Type: "A", Content: "203.0.113.1", TTL: 300})
	if err != nil || created.ID == "" {
		t.Fatalf("create: %v %+v", err, created)
	}
	if recs := fake.Records(); len(recs) != 1 || recs[0].Comment != managedComment {
		t.Fatalf("fake: %+v", recs)
	}

	got, err := c.GetRecord(ctx, "z1", "home.example.com", "A")
	if err != nil || got.Content != "203.0.113.1" || got.TTL != 300 || got.ID != created.ID {
		t.Fatalf("get: %v %+v", err, got)
	}

	got.Content, got.Proxied = "203.0.113.2", true
	upd, err := c.UpdateRecord(ctx, got)
	if err != nil || upd.Content != "203.0.113.2" || !upd.Proxied || upd.TTL != 1 {
		t.Fatalf("update: %v %+v", err, upd)
	}
	if fake.Creates.Load() != 1 || fake.Patches.Load() != 1 {
		t.Fatalf("creates=%d patches=%d", fake.Creates.Load(), fake.Patches.Load())
	}
}

func TestMultipleRecords(t *testing.T) {
	c, fake := setup(t)
	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		fake.AddRecord(cftest.Record{ZoneID: "z1", Name: "rr.example.com", Type: "A", Content: ip, TTL: 1})
	}
	_, err := c.GetRecord(context.Background(), "z1", "rr.example.com", "A")
	if err == nil || !strings.Contains(err.Error(), "2 A-Einträge") {
		t.Fatalf("Fehler erwartet, bekam %v", err)
	}
}

func TestErrors(t *testing.T) {
	c, fake := setup(t)
	ctx := context.Background()

	// API-Fehler wird mit Meldung durchgereicht
	_, err := c.UpdateRecord(ctx, providers.Record{Zone: "z1", Name: "x.example.com", Type: "A", Content: "kaputt", TTL: 1})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("APIError erwartet, bekam %v", err)
	}

	fake.Fail(500)
	if _, err := c.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("HTTP 500 erwartet, bekam %v", err)
	}

	// falsches Token: Fehlermeldung darf das Token nicht enthalten
	bad := New("geheimes-falsches-token", c.baseURL)
	_, err = bad.ListZones(ctx)
	if err == nil || strings.Contains(err.Error(), "geheimes") {
		t.Fatalf("unerwartet: %v", err)
	}

	// kein Token
	if _, err := New("", c.baseURL).ListZones(ctx); !errors.Is(err, providers.ErrNotConfigured) {
		t.Fatalf("ErrNotConfigured erwartet, bekam %v", err)
	}
}

func TestListTunnels(t *testing.T) {
	c, fake := setup(t)
	ctx := context.Background()
	fake.SetAccount("acc1")
	for i := range 55 { // > 1 Seite (per_page=50)
		fake.AddTunnel(fmt.Sprintf("t%d", i), fmt.Sprintf("tunnel-%d", i), "healthy")
	}
	fake.SetTunnelStatus("t1", "down")

	tunnels, err := c.ListTunnels(ctx, "acc1")
	if err != nil || len(tunnels) != 55 {
		t.Fatalf("%v, %d Tunnels", err, len(tunnels))
	}
	if tunnels[0].Status != "healthy" || len(tunnels[0].Connections) != 4 || tunnels[0].Connections[0].ColoName != "fra06" {
		t.Fatalf("t0: %+v", tunnels[0])
	}
	if tunnels[1].Status != "down" || len(tunnels[1].Connections) != 0 || tunnels[1].ConnsInactiveAt == nil {
		t.Fatalf("t1: %+v", tunnels[1])
	}

	// falsche Account-ID → verständlicher Rechte-Fehler
	if _, err := c.ListTunnels(ctx, "falsch"); !errors.Is(err, ErrTunnelPermission) {
		t.Fatalf("ErrTunnelPermission erwartet, bekam %v", err)
	}
}
