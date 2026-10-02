package store

import (
	"context"
	"errors"
	"testing"
)

func TestProbes(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	p, err := s.CreateProbe(ctx, "https://a.example/", true)
	if err != nil || p.Status != ProbePending || p.RecordID != nil {
		t.Fatalf("anlegen: %+v %v", p, err)
	}
	if _, err := s.CreateProbe(ctx, "https://a.example/", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("doppelt: %v", err)
	}
	if p, err = s.UpdateProbe(ctx, p.ID, "https://a.example/", false); err != nil || p.Status != ProbePaused {
		t.Fatalf("deaktivieren: %+v %v", p, err)
	}
	// Ergebnisse deaktivierter Prüfungen werden nicht gespeichert
	if err := s.SetProbeResult(ctx, p.ID, p.URL, ProbeResult{Status: ProbeUp}); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.GetProbe(ctx, p.ID); p.Status != ProbePaused {
		t.Fatalf("Ergebnis trotz Pause: %+v", p)
	}

	// Record-Prüfung: eigene Prüfung derselben Adresse ist erlaubt
	rec, err := s.CreateRecord(ctx, Record{ZoneID: "z", ZoneName: "example", Name: "a.example", Type: "A", TTL: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRecordProbe(ctx, rec.ID, "https://a.example/"); err != nil {
		t.Fatal(err)
	}
	rp, err := s.RecordProbe(ctx, rec.ID)
	if err != nil || rp.RecordID == nil || *rp.RecordID != rec.ID || rp.RecordName != "a.example" {
		t.Fatalf("Record-Prüfung: %+v %v", rp, err)
	}
	// erneut setzen ist idempotent, neue Adresse beginnt von vorn
	s.SetProbeResult(ctx, rp.ID, rp.URL, ProbeResult{Status: ProbeDown, FailCount: 3})
	s.SetRecordProbe(ctx, rec.ID, "https://a.example/")
	if rp, _ = s.RecordProbe(ctx, rec.ID); rp.Status != ProbeDown || rp.FailCount != 3 {
		t.Fatalf("idempotent: %+v", rp)
	}
	s.SetRecordProbe(ctx, rec.ID, "https://b.example/")
	if rp, _ = s.RecordProbe(ctx, rec.ID); rp.Status != ProbePending || rp.FailCount != 0 || rp.URL != "https://b.example/" {
		t.Fatalf("neue Adresse: %+v", rp)
	}

	list, _ := s.ListProbes(ctx)
	if len(list) != 2 {
		t.Fatalf("Liste: %+v", list)
	}

	// Record löschen entfernt seine Prüfung
	if err := s.DeleteRecord(ctx, rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordProbe(ctx, rec.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nach Löschen des Records: %v", err)
	}
	if err := s.DeleteProbe(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProbe(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("doppelt gelöscht: %v", err)
	}
}
