package api

import (
	"context"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/speedtest"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

// fakeSpeed misst sofort; mit gate wartet die Messung, bis gate geschlossen ist.
type fakeSpeed struct{ gate chan struct{} }

func (f *fakeSpeed) Run(ctx context.Context) (speedtest.Result, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return speedtest.Result{}, ctx.Err()
		}
	}
	return speedtest.Result{DownloadMbps: 250, UploadMbps: 40, LatencyMS: 12, JitterMS: 1,
		Colo: "FRA", City: "Augsburg", Country: "DE", IP: "203.0.113.7"}, nil
}

func TestSpeedtestAuthRequired(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{{"GET", "/api/speedtest"}, {"POST", "/api/speedtest/run"}} {
		if resp := e.do(t, p.m, p.path, "", nil); resp.StatusCode != 401 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
}

func TestSpeedtestRun(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	got := decode[speedtestDTO](t, e.do(t, "GET", "/api/speedtest", "", c))
	if !got.Available || got.Running || len(got.Results) != 0 || got.IntervalSeconds != 0 {
		t.Fatalf("leer: %+v", got)
	}

	gate := make(chan struct{})
	e.speedFake.gate = gate
	if resp := e.do(t, "POST", "/api/speedtest/run", "", c); resp.StatusCode != 202 {
		t.Fatalf("starten: %d", resp.StatusCode)
	}
	if got = decode[speedtestDTO](t, e.do(t, "GET", "/api/speedtest", "", c)); !got.Running || got.RunningAt == nil {
		t.Fatalf("läuft nicht: %+v", got)
	}
	resp := e.do(t, "POST", "/api/speedtest/run", "", c)
	if msg := decode[map[string]any](t, resp); resp.StatusCode != 409 || msg["code"] != "speedtest.running" {
		t.Fatalf("doppelt: %d %v", resp.StatusCode, msg)
	}
	close(gate)
	e.speed.Wait()

	got = decode[speedtestDTO](t, e.do(t, "GET", "/api/speedtest", "", c))
	if got.Running || len(got.Results) != 1 {
		t.Fatalf("Ergebnis: %+v", got)
	}
	r := got.Results[0]
	if r.Trigger != store.TriggerManual || *r.DownloadMbps != 250 || r.Colo != "FRA" || r.City != "Augsburg" || r.Error != "" {
		t.Fatalf("Messwerte: %+v", r)
	}
}

func TestSpeedtestSettings(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	body := `{"ip_check_interval_seconds":300,"tunnel_interval_seconds":60,"notify_language":"de",
		"ip_sources":[{"name":"cloudflare","enabled":true},{"name":"ipify","enabled":true}]`
	if resp := e.do(t, "PUT", "/api/settings", body+`,"speedtest_interval_seconds":60}`, c); resp.StatusCode != 400 {
		t.Fatalf("zu kurz: %d", resp.StatusCode)
	}
	s := decode[settingsDTO](t, e.do(t, "PUT", "/api/settings", body+`,"speedtest_interval_seconds":21600}`, c))
	if s.SpeedtestIntervalSeconds != 21600 || s.Limits["speedtest_interval_min"] != 3600 {
		t.Fatalf("gespeichert: %+v", s)
	}
	if e.speed.Interval() != 6*time.Hour {
		t.Fatalf("Runner sieht %v", e.speed.Interval())
	}
	// fehlt = unverändert, 0 = aus
	if s = decode[settingsDTO](t, e.do(t, "PUT", "/api/settings", body+`}`, c)); s.SpeedtestIntervalSeconds != 21600 {
		t.Fatalf("unverändert: %+v", s)
	}
	if s = decode[settingsDTO](t, e.do(t, "PUT", "/api/settings", body+`,"speedtest_interval_seconds":0}`, c)); s.SpeedtestIntervalSeconds != 0 {
		t.Fatalf("aus: %+v", s)
	}
}
