package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

func TestSpeedtests(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	if _, err := s.LastSpeedtest(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("leer: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	down, up, lat := 250.5, 40.25, 12.0
	if _, err := s.InsertSpeedtest(ctx, Speedtest{StartedAt: old, DurationMS: 18000, Trigger: TriggerScheduled,
		DownloadMbps: &down, UploadMbps: &up, LatencyMS: &lat, Colo: "FRA", Country: "DE", IP: "203.0.113.7",
		DownloadBytes: 1 << 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertSpeedtest(ctx, Speedtest{StartedAt: time.Now(), Trigger: TriggerManual,
		ErrorMsg: i18n.M("speedtest.timeout")}); err != nil {
		t.Fatal(err)
	}

	last, err := s.LastSpeedtest(ctx)
	if err != nil || last.Trigger != TriggerManual || last.DownloadMbps != nil || last.ErrorMsg.Code != "speedtest.timeout" {
		t.Fatalf("letzte: %+v %v", last, err)
	}
	list, err := s.ListSpeedtests(ctx, 0, 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("Liste: %+v %v", list, err)
	}
	ok := list[1]
	if *ok.DownloadMbps != down || *ok.UploadMbps != up || *ok.LatencyMS != lat || ok.JitterMS != nil ||
		ok.Colo != "FRA" || ok.City != "" || ok.DownloadBytes != 1<<20 || !ok.StartedAt.Equal(old.UTC().Truncate(time.Nanosecond)) {
		t.Fatalf("Messwerte: %+v", ok)
	}
	if page, _ := s.ListSpeedtests(ctx, list[0].ID, 10); len(page) != 1 || page[0].ID != ok.ID {
		t.Fatalf("Blättern: %+v", page)
	}

	if n, err := s.PruneSpeedtests(ctx, time.Now().Add(-24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("aufräumen: %d %v", n, err)
	}
}
