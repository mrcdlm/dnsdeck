package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecordsCRUD(t *testing.T) {
	s, ctx := openTest(t), context.Background()

	r, err := s.CreateRecord(ctx, Record{ZoneID: "z1", ZoneName: "example.com",
		Name: "home.example.com", Type: "A", TTL: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == 0 || r.Status != RecordPending || r.Provider != "cloudflare" {
		t.Fatalf("unerwartet: %+v", r)
	}

	if _, err := s.CreateRecord(ctx, Record{ZoneID: "z1", ZoneName: "example.com",
		Name: "home.example.com", Type: "A", TTL: 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("ErrConflict erwartet, bekam %v", err)
	}

	now := time.Now()
	if err := s.SetRecordSyncState(ctx, r.ID, RecordSyncState{ProviderRecordID: "cf1",
		CurrentIP: "203.0.113.1", Status: RecordOK, CheckedAt: now, Changed: true}); err != nil {
		t.Fatal(err)
	}
	// Fehler behält IP und Provider-ID
	if err := s.SetRecordSyncState(ctx, r.ID, RecordSyncState{Status: RecordError,
		Message: "kaputt", CheckedAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	r, _ = s.GetRecord(ctx, r.ID)
	if r.ProviderRecordID != "cf1" || r.CurrentIP != "203.0.113.1" || r.Status != RecordError ||
		r.Message != "kaputt" || r.LastChangedAt == nil || !r.LastChangedAt.Equal(now) {
		t.Fatalf("Sync-State: %+v", r)
	}

	// Nur TTL geändert → Provider-ID bleibt
	r.TTL = 300
	r, err = s.UpdateRecordSettings(ctx, r)
	if err != nil || r.ProviderRecordID != "cf1" || r.TTL != 300 || r.Status != RecordPending || r.Message != "" {
		t.Fatalf("Settings: %v %+v", err, r)
	}
	// Name geändert → Provider-ID verworfen; deaktiviert → paused
	r.Name, r.Enabled = "nas.example.com", false
	r, _ = s.UpdateRecordSettings(ctx, r)
	if r.ProviderRecordID != "" || r.Status != RecordPaused {
		t.Fatalf("Namenswechsel: %+v", r)
	}

	list, _ := s.ListRecords(ctx)
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}

	// Log bleibt nach Löschen erhalten, record_id wird NULL
	id := r.ID
	s.InsertUpdateLog(ctx, UpdateLogEntry{RecordID: &id, RecordName: r.Name, RecordType: "A",
		Trigger: TriggerManual, Result: ResultUpdated, OldIP: "203.0.113.1", NewIP: "203.0.113.2",
		CreatedAt: now})
	if err := s.DeleteRecord(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRecord(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ErrNotFound erwartet, bekam %v", err)
	}
	log, err := s.ListUpdateLog(ctx, UpdateLogFilter{}, 10)
	if err != nil || len(log) != 1 || log[0].RecordID != nil || log[0].NewIP != "203.0.113.2" {
		t.Fatalf("Log: %v %+v", err, log)
	}
}

func TestUpdateLogFilter(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	a, _ := s.CreateRecord(ctx, Record{ZoneID: "z", ZoneName: "e.com", Name: "a.e.com", Type: "A", TTL: 1, Enabled: true})
	b, _ := s.CreateRecord(ctx, Record{ZoneID: "z", ZoneName: "e.com", Name: "b.e.com", Type: "A", TTL: 1, Enabled: true})
	for _, id := range []int64{a.ID, b.ID, a.ID} {
		s.InsertUpdateLog(ctx, UpdateLogEntry{RecordID: &id, RecordName: "x", RecordType: "A",
			Trigger: TriggerScheduled, Result: ResultError, Message: "m", CreatedAt: time.Now()})
	}
	if l, _ := s.ListUpdateLog(ctx, UpdateLogFilter{RecordID: a.ID}, 10); len(l) != 2 {
		t.Fatalf("Filter a: %d", len(l))
	}
	if l, _ := s.ListUpdateLog(ctx, UpdateLogFilter{}, 2); len(l) != 2 {
		t.Fatalf("Limit: %d", len(l))
	}
}

func TestSettingsPending(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	r, _ := s.CreateRecord(ctx, Record{ZoneID: "z", ZoneName: "e.com", Name: "a.e.com", Type: "A", TTL: 1, Enabled: true})
	if r.SettingsPending {
		t.Fatal("neuer Record darf nicht pending sein")
	}
	yes, no, auto, fiveMin := true, false, 1, 300
	get := func() Record {
		t.Helper()
		got, err := s.GetRecord(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	save := func(r Record) Record {
		t.Helper()
		got, err := s.UpdateRecordSettings(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	sync := func(p *bool, ttl *int) Record {
		t.Helper()
		if err := s.SetRecordSyncState(ctx, r.ID, RecordSyncState{Status: RecordOK, CheckedAt: time.Now(), Proxied: p, TTL: ttl}); err != nil {
			t.Fatal(err)
		}
		return get()
	}

	// Cloudflare-Werte werden übernommen, solange nichts aussteht
	if got := sync(&yes, &auto); !got.Proxied || got.SettingsPending {
		t.Fatalf("Übernahme: %+v", got)
	}

	// Nur "enabled" geändert → nicht pending
	r = get()
	r.Enabled = false
	if got := save(r); got.SettingsPending {
		t.Fatal("pending ohne Proxy/TTL-Änderung")
	}

	// TTL in dnsdeck geändert → pending; Cloudflare-Stand (alt) überschreibt nicht
	r.Enabled, r.Proxied, r.TTL = true, false, 300
	if got := save(r); !got.SettingsPending {
		t.Fatal("pending erwartet")
	}
	if got := sync(&yes, &auto); got.Proxied || got.TTL != 300 || !got.SettingsPending {
		t.Fatalf("ausstehende Änderung überschrieben: %+v", got)
	}
	// Fehler (keine Werte) ändert nichts
	if got := sync(nil, nil); !got.SettingsPending {
		t.Fatal("pending durch Fehler verloren")
	}
	// Übertragen → pending gelöscht
	if got := sync(&no, &fiveMin); got.SettingsPending || got.TTL != 300 {
		t.Fatalf("nach Übertragung: %+v", got)
	}
}
