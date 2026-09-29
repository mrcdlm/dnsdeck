package ddns

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare"
	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare/cftest"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

type env struct {
	st   *store.Store
	fake *cftest.Fake
	u    *Updater
}

func setup(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fake := cftest.New("tok", cftest.Zone{ID: "z1", Name: "example.com"})
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	u := NewUpdater(st, cloudflare.New("tok", srv.URL), slog.New(slog.DiscardHandler))
	return &env{st: st, fake: fake, u: u}
}

func (e *env) add(t *testing.T, name, typ string, ttl int, proxied bool) store.Record {
	t.Helper()
	r, err := e.st.CreateRecord(context.Background(), store.Record{ZoneID: "z1", ZoneName: "example.com",
		Name: name, Type: typ, TTL: ttl, Proxied: proxied, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *env) get(t *testing.T, id int64) store.Record {
	t.Helper()
	r, err := e.st.GetRecord(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *env) logs(t *testing.T) []store.UpdateLogEntry {
	t.Helper()
	l, err := e.st.ListUpdateLog(context.Background(), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

var ips1 = IPs{V4: "203.0.113.1", V6: "2001:db8::1"}

func TestCreatesMissingRecord(t *testing.T) {
	e, ctx := setup(t), context.Background()
	r := e.add(t, "home.example.com", "A", 300, false)

	if err := e.u.SyncAll(ctx, ips1, store.TriggerScheduled); err != nil {
		t.Fatal(err)
	}
	recs := e.fake.Records()
	if len(recs) != 1 || recs[0].Content != "203.0.113.1" || recs[0].TTL != 300 {
		t.Fatalf("Cloudflare: %+v", recs)
	}
	got := e.get(t, r.ID)
	if got.Status != store.RecordOK || got.CurrentIP != "203.0.113.1" || got.LastChangedAt == nil {
		t.Fatalf("Record: %+v", got)
	}
	if l := e.logs(t); len(l) != 1 || l[0].Result != store.ResultCreated || l[0].NewIP != "203.0.113.1" {
		t.Fatalf("Log: %+v", l)
	}
}

func TestNoUpdateWhenUnchanged(t *testing.T) {
	e, ctx := setup(t), context.Background()
	e.fake.AddRecord(cftest.Record{ZoneID: "z1", Name: "home.example.com", Type: "A", Content: "203.0.113.1", TTL: 1})
	r := e.add(t, "home.example.com", "A", 1, false)

	for range 3 {
		e.u.SyncAll(ctx, ips1, store.TriggerScheduled)
	}
	if e.fake.Patches.Load() != 0 || e.fake.Creates.Load() != 0 {
		t.Fatalf("unnötige Änderung: patches=%d creates=%d", e.fake.Patches.Load(), e.fake.Creates.Load())
	}
	if e.fake.Gets.Load() != 3 {
		t.Fatalf("Ist-Zustand nicht jedes Mal abgefragt: %d", e.fake.Gets.Load())
	}
	if got := e.get(t, r.ID); got.Status != store.RecordOK || got.LastChangedAt != nil {
		t.Fatalf("Record: %+v", got)
	}
	// genau ein Eintrag: die Übernahme beim ersten Abgleich
	if l := e.logs(t); len(l) != 1 || l[0].Result != store.ResultAdopted ||
		l[0].Message != "bestehenden Eintrag übernommen (Proxy aus, TTL Auto)" {
		t.Fatalf("Log: %+v", l)
	}
}

func TestUpdatesOnIPChange(t *testing.T) {
	e, ctx := setup(t), context.Background()
	e.fake.AddRecord(cftest.Record{ZoneID: "z1", Name: "home.example.com", Type: "A", Content: "203.0.113.9", TTL: 1})
	e.add(t, "home.example.com", "A", 1, false)

	e.u.SyncAll(ctx, ips1, store.TriggerScheduled)
	if recs := e.fake.Records(); recs[0].Content != "203.0.113.1" {
		t.Fatalf("nicht aktualisiert: %+v", recs)
	}
	l := e.logs(t)
	if len(l) != 1 || l[0].Result != store.ResultUpdated || l[0].OldIP != "203.0.113.9" || l[0].NewIP != "203.0.113.1" {
		t.Fatalf("Log: %+v", l)
	}
}

// Bestehender Eintrag: Proxy/TTL kommen von Cloudflare, nicht aus dem
// (voreingestellten) Dialog; geändert wird nur die IP.
func TestAdoptKeepsCloudflareSettings(t *testing.T) {
	e, ctx := setup(t), context.Background()
	e.fake.AddRecord(cftest.Record{ZoneID: "z1", Name: "immich.example.com", Type: "A", Content: "1.2.3.4", TTL: 1, Proxied: true})
	r := e.add(t, "immich.example.com", "A", 1, false) // Dialog-Voreinstellung: Proxy aus

	e.u.SyncAll(ctx, ips1, store.TriggerRecordSaved)
	cf := e.fake.Records()[0]
	if !cf.Proxied || cf.Content != "203.0.113.1" {
		t.Fatalf("Proxy darf nicht abgeschaltet werden: %+v", cf)
	}
	if got := e.get(t, r.ID); !got.Proxied || got.SettingsPending {
		t.Fatalf("dnsdeck muss Proxy übernehmen: %+v", got)
	}
	l := e.logs(t)
	if len(l) != 1 || l[0].Result != store.ResultUpdated || l[0].OldIP != "1.2.3.4" ||
		l[0].Message != "bestehenden Eintrag übernommen" {
		t.Fatalf("Log: %+v", l)
	}
}

func TestSettingsChangedInDnsdeckArePushed(t *testing.T) {
	e, ctx := setup(t), context.Background()
	e.fake.AddRecord(cftest.Record{ZoneID: "z1", Name: "home.example.com", Type: "A", Content: "203.0.113.1", TTL: 1})
	r := e.add(t, "home.example.com", "A", 1, false)
	e.u.SyncAll(ctx, ips1, store.TriggerScheduled)

	r = e.get(t, r.ID)
	r.TTL = 300
	e.st.UpdateRecordSettings(ctx, r)
	e.u.SyncRecord(ctx, r.ID, ips1, store.TriggerRecordSaved)

	if cf := e.fake.Records()[0]; cf.TTL != 300 {
		t.Fatalf("TTL nicht übertragen: %+v", cf)
	}
	if got := e.get(t, r.ID); got.SettingsPending {
		t.Fatal("pending nach Übertragung")
	}
	l := e.logs(t)
	if l[0].Result != store.ResultUpdated || l[0].Message != "TTL: Auto → 5 min" {
		t.Fatalf("Log: %+v", l[0])
	}
	// weitere Läufe: nichts mehr zu tun
	e.u.SyncAll(ctx, ips1, store.TriggerScheduled)
	if e.fake.Patches.Load() != 1 {
		t.Fatalf("patches = %d", e.fake.Patches.Load())
	}
}

func TestSettingsChangedAtCloudflareAreAdopted(t *testing.T) {
	e, ctx := setup(t), context.Background()
	id := e.fake.AddRecord(cftest.Record{ZoneID: "z1", Name: "home.example.com", Type: "A", Content: "203.0.113.1", TTL: 1})
	r := e.add(t, "home.example.com", "A", 1, false)
	e.u.SyncAll(ctx, ips1, store.TriggerScheduled)

	// Im Cloudflare-Dashboard: Proxy an und zugleich falsche IP
	e.fake.Patch(id, func(c *cftest.Record) { c.Proxied, c.Content = true, "1.2.3.4" })
	e.u.SyncAll(ctx, ips1, store.TriggerManual)

	cf := e.fake.Records()[0]
	if !cf.Proxied || cf.Content != "203.0.113.1" {
		t.Fatalf("Proxy muss bleiben, IP korrigiert werden: %+v", cf)
	}
	if got := e.get(t, r.ID); !got.Proxied {
		t.Fatalf("dnsdeck muss Proxy übernehmen: %+v", got)
	}
	l := e.logs(t)
	if l[0].OldIP != "1.2.3.4" || l[0].NewIP != "203.0.113.1" || l[0].Message != "" {
		t.Fatalf("Log: %+v", l[0])
	}
}

func TestAAAAAndSkipped(t *testing.T) {
	e, ctx := setup(t), context.Background()
	v6 := e.add(t, "home.example.com", "AAAA", 1, false)

	e.u.SyncAll(ctx, IPs{V4: "203.0.113.1"}, store.TriggerScheduled)
	if got := e.get(t, v6.ID); got.Status != store.RecordSkipped {
		t.Fatalf("skipped erwartet: %+v", got)
	}
	if len(e.fake.Records()) != 0 {
		t.Fatal("ohne IPv6 darf nichts angelegt werden")
	}

	e.u.SyncAll(ctx, ips1, store.TriggerScheduled)
	if recs := e.fake.Records(); len(recs) != 1 || recs[0].Content != "2001:db8::1" {
		t.Fatalf("AAAA: %+v", recs)
	}
}

func TestErrorsAreDeduplicated(t *testing.T) {
	e, ctx := setup(t), context.Background()
	r := e.add(t, "home.example.com", "A", 1, false)
	e.fake.Fail(500)

	for range 3 {
		e.u.SyncAll(ctx, ips1, store.TriggerScheduled)
	}
	got := e.get(t, r.ID)
	if got.Status != store.RecordError || got.Message == "" {
		t.Fatalf("Record: %+v", got)
	}
	if l := e.logs(t); len(l) != 1 || l[0].Result != store.ResultError {
		t.Fatalf("genau ein Fehlereintrag erwartet: %+v", l)
	}
	// manueller Versuch wird immer protokolliert
	e.u.SyncAll(ctx, ips1, store.TriggerManual)
	if l := e.logs(t); len(l) != 2 {
		t.Fatalf("manueller Fehler fehlt: %d", len(l))
	}

	// Erholung
	e.fake.Fail(0)
	e.u.SyncAll(ctx, ips1, store.TriggerScheduled)
	if got := e.get(t, r.ID); got.Status != store.RecordOK || got.Message != "" {
		t.Fatalf("nach Erholung: %+v", got)
	}
}

func TestDisabledAndNotConfigured(t *testing.T) {
	e, ctx := setup(t), context.Background()
	r := e.add(t, "home.example.com", "A", 1, false)
	r.Enabled = false
	e.st.UpdateRecordSettings(ctx, r)

	e.u.SyncAll(ctx, ips1, store.TriggerScheduled)
	if got := e.get(t, r.ID); got.Status != store.RecordPaused || len(e.fake.Records()) != 0 {
		t.Fatalf("deaktiviert: %+v", got)
	}

	r.Enabled = true
	e.st.UpdateRecordSettings(ctx, r)
	u := NewUpdater(e.st, nil, slog.New(slog.DiscardHandler))
	got, err := u.SyncRecord(ctx, r.ID, ips1, store.TriggerManual)
	if err != nil || got.Status != store.RecordError || got.Message == "" {
		t.Fatalf("ohne Provider: %v %+v", err, got)
	}
}
