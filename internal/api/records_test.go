package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRecordsAuthRequired(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{
		{"GET", "/api/zones"}, {"GET", "/api/records"}, {"POST", "/api/records"},
		{"PUT", "/api/records/1"}, {"DELETE", "/api/records/1"},
		{"POST", "/api/records/1/sync"}, {"POST", "/api/records/sync"}, {"GET", "/api/updates"},
	} {
		if resp := e.do(t, p.m, p.path, "", nil); resp.StatusCode != 401 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
}

func TestRecordLifecycle(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	zones := decode[[]map[string]string](t, e.do(t, "GET", "/api/zones", "", c))
	if len(zones) != 1 || zones[0]["name"] != "example.com" {
		t.Fatalf("zones: %+v", zones)
	}

	// Anlegen → sofortiger Abgleich legt Eintrag bei Cloudflare an
	resp := e.do(t, "POST", "/api/records", `{"zone_id":"z1","name":"Home.Example.com.","type":"A","ttl":300}`, c)
	if resp.StatusCode != 201 {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	rec := decode[store.Record](t, resp)
	if rec.Name != "home.example.com" || rec.Status != store.RecordOK || rec.CurrentIP != "203.0.113.1" || !rec.Enabled {
		t.Fatalf("record: %+v", rec)
	}
	if cf := e.cf.Records(); len(cf) != 1 || cf[0].Content != "203.0.113.1" || cf[0].TTL != 300 {
		t.Fatalf("cloudflare: %+v", cf)
	}

	// Duplikat
	if resp := e.do(t, "POST", "/api/records", `{"zone_id":"z1","name":"home.example.com","type":"A"}`, c); resp.StatusCode != 409 {
		t.Fatalf("Duplikat: %d", resp.StatusCode)
	}

	// Ändern: proxied → TTL wird automatisch
	path := fmt.Sprintf("/api/records/%d", rec.ID)
	resp = e.do(t, "PUT", path, `{"zone_id":"z1","name":"home.example.com","type":"A","ttl":300,"proxied":true}`, c)
	rec = decode[store.Record](t, resp)
	if !rec.Proxied || rec.TTL != 1 || rec.Status != store.RecordOK {
		t.Fatalf("update: %+v", rec)
	}
	if cf := e.cf.Records(); !cf[0].Proxied {
		t.Fatalf("cloudflare nicht aktualisiert: %+v", cf)
	}

	// Update-Log: angelegt + Proxy geändert
	log := decode[[]store.UpdateLogEntry](t, e.do(t, "GET", "/api/updates", "", c))
	if len(log) != 2 || log[0].Result != store.ResultUpdated || log[1].Result != store.ResultCreated ||
		log[1].Trigger != store.TriggerRecordSaved {
		t.Fatalf("log: %+v", log)
	}

	// Manueller Gesamt-Abgleich
	resp = e.do(t, "POST", "/api/records/sync", "", c)
	if resp.StatusCode != 200 || e.tracker.checks != 1 {
		t.Fatalf("sync all: %d checks=%d", resp.StatusCode, e.tracker.checks)
	}

	// Löschen entfernt nur aus dnsdeck, nicht bei Cloudflare
	if resp := e.do(t, "DELETE", path, "", c); resp.StatusCode != 204 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp := e.do(t, "DELETE", path, "", c); resp.StatusCode != 404 {
		t.Fatalf("delete 2: %d", resp.StatusCode)
	}
	if len(e.cf.Records()) != 1 {
		t.Fatal("Eintrag bei Cloudflare wurde gelöscht")
	}
	if recs := decode[[]store.Record](t, e.do(t, "GET", "/api/records", "", c)); len(recs) != 0 {
		t.Fatalf("records: %+v", recs)
	}
}

func TestRecordValidation(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	for _, body := range []string{
		`{"zone_id":"zX","name":"home.example.com","type":"A"}`,          // Zone unbekannt
		`{"zone_id":"z1","name":"home.example.org","type":"A"}`,          // falsche Zone
		`{"zone_id":"z1","name":"-bad.example.com","type":"A"}`,          // ungültiges Label
		`{"zone_id":"z1","name":"a..example.com","type":"A"}`,            // leeres Label
		`{"zone_id":"z1","name":"home.example.com","type":"CNAME"}`,      // Typ
		`{"zone_id":"z1","name":"home.example.com","type":"A","ttl":30}`, // TTL
		`{"zone_id":"z1","name":"home.example.com","type":"A","x":1}`,    // unbekanntes Feld
		`kein json`,
	} {
		if resp := e.do(t, "POST", "/api/records", body, c); resp.StatusCode != 400 {
			t.Errorf("%s: %d", body, resp.StatusCode)
		}
	}
	// Apex über "@"
	resp := e.do(t, "POST", "/api/records", `{"zone_id":"z1","name":"@","type":"A"}`, c)
	if rec := decode[store.Record](t, resp); resp.StatusCode != 201 || rec.Name != "example.com" {
		t.Fatalf("apex: %d %+v", resp.StatusCode, rec)
	}
}

func TestRecordCloudflareErrorShownInStatus(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	resp := e.do(t, "POST", "/api/records", `{"zone_id":"z1","name":"home.example.com","type":"A"}`, c)
	rec := decode[store.Record](t, resp)

	e.cf.Fail(500)
	resp = e.do(t, "POST", fmt.Sprintf("/api/records/%d/sync", rec.ID), "", c)
	rec = decode[store.Record](t, resp)
	if resp.StatusCode != 200 || rec.Status != store.RecordError || rec.Message == "" {
		t.Fatalf("%d %+v", resp.StatusCode, rec)
	}
	// Zonen nicht ladbar → 502
	if resp := e.do(t, "GET", "/api/zones", "", c); resp.StatusCode != 502 {
		t.Fatalf("zones: %d", resp.StatusCode)
	}
}

func TestZonesNotConfigured(t *testing.T) {
	st, _ := store.Open(context.Background(), ":memory:")
	t.Cleanup(func() { st.Close() })
	auth, _ := NewAuth("pw", st)
	s := NewServer(Deps{Store: st, Tracker: fakeTracker{}, Auth: auth,
		Log: slog.New(slog.DiscardHandler), Static: fstest.MapFS{}})
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)
	e := &testEnv{srv: srv, auth: auth}
	resp := e.do(t, "POST", "/api/login", `{"password":"pw"}`, nil)
	c := resp.Cookies()[0]
	if resp := e.do(t, "GET", "/api/zones", "", c); resp.StatusCode != 503 {
		t.Fatalf("zones: %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", "/api/records", `{"zone_id":"z1","name":"a.example.com","type":"A"}`, c); resp.StatusCode != 503 {
		t.Fatalf("create: %d", resp.StatusCode)
	}
}
