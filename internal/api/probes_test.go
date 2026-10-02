package api

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/probe"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

// fakeChecker: Adressen mit "down" sind nicht erreichbar, alle anderen antworten mit 200.
type fakeChecker struct{}

func (fakeChecker) Check(_ context.Context, url string) probe.Result {
	if strings.Contains(url, "down") {
		return probe.Result{Err: i18n.M("probe.timeout")}
	}
	return probe.Result{HTTPStatus: 200, Latency: 10 * time.Millisecond,
		TLS: &probe.TLSInfo{NotAfter: time.Now().Add(60 * 24 * time.Hour), Issuer: "Test CA"}}
}

func TestProbesAuthRequired(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{
		{"GET", "/api/probes"}, {"POST", "/api/probes"}, {"PUT", "/api/probes/1"},
		{"DELETE", "/api/probes/1"}, {"POST", "/api/probes/1/run"},
	} {
		if resp := e.do(t, p.m, p.path, "", nil); resp.StatusCode != 401 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
}

func TestProbeLifecycle(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	resp := e.do(t, "POST", "/api/probes", `{"url":"ftp://x"}`, c)
	if msg := decode[map[string]any](t, resp); resp.StatusCode != 400 || msg["code"] != "probe.url_invalid" {
		t.Fatalf("ungültig: %d %v", resp.StatusCode, msg)
	}

	// anlegen prüft sofort
	resp = e.do(t, "POST", "/api/probes", `{"url":"NAS.example.com"}`, c)
	p := decode[store.Probe](t, resp)
	if resp.StatusCode != 201 || p.URL != "https://nas.example.com/" || p.Status != store.ProbeUp ||
		p.HTTPStatus == nil || *p.HTTPStatus != 200 || p.TLSIssuer != "Test CA" || p.RecordID != nil {
		t.Fatalf("anlegen: %d %+v", resp.StatusCode, p)
	}
	if resp := e.do(t, "POST", "/api/probes", `{"url":"https://nas.example.com"}`, c); resp.StatusCode != 409 {
		t.Fatalf("doppelt: %d", resp.StatusCode)
	}

	// Adresse ändern → neu geprüft; erster Fehlschlag gilt sofort
	resp = e.do(t, "PUT", "/api/probes/"+itoa(p.ID), `{"url":"https://down.example.com"}`, c)
	p = decode[store.Probe](t, resp)
	if resp.StatusCode != 200 || p.Status != store.ProbeDown || p.Message != "Zeitüberschreitung – keine Antwort" {
		t.Fatalf("ändern: %d %+v", resp.StatusCode, p)
	}

	// deaktivieren → pausiert, „Jetzt prüfen“ ändert nichts
	p = decode[store.Probe](t, e.do(t, "PUT", "/api/probes/"+itoa(p.ID), `{"enabled":false}`, c))
	if p.Status != store.ProbePaused || p.Enabled {
		t.Fatalf("deaktivieren: %+v", p)
	}
	if p = decode[store.Probe](t, e.do(t, "POST", "/api/probes/"+itoa(p.ID)+"/run", "", c)); p.Status != store.ProbePaused {
		t.Fatalf("Prüfen trotz Pause: %+v", p)
	}

	if resp := e.do(t, "DELETE", "/api/probes/"+itoa(p.ID), "", c); resp.StatusCode != 204 {
		t.Fatalf("löschen: %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", "/api/probes/"+itoa(p.ID)+"/run", "", c); resp.StatusCode != 404 {
		t.Fatalf("nach Löschen: %d", resp.StatusCode)
	}
}

func TestRecordProbe(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	rec := decode[store.Record](t, e.do(t, "POST", "/api/records", `{"zone_id":"z1","name":"nas.example.com","type":"A","probe":true}`, c))
	list := decode[[]store.Probe](t, e.do(t, "GET", "/api/probes", "", c))
	if len(list) != 1 || list[0].URL != "https://nas.example.com/" || list[0].RecordName != "nas.example.com" ||
		list[0].RecordID == nil || *list[0].RecordID != rec.ID {
		t.Fatalf("Record-Prüfung: %+v", list)
	}

	// Adresse folgt dem Record und kann nicht separat geändert werden
	if resp := e.do(t, "PUT", "/api/probes/"+itoa(list[0].ID), `{"url":"https://anders.example.com"}`, c); resp.StatusCode != 400 {
		t.Fatalf("Adresse ändern: %d", resp.StatusCode)
	}
	e.do(t, "PUT", "/api/records/"+itoa(rec.ID), `{"zone_id":"z1","name":"nas2.example.com","type":"A"}`, c)
	list = decode[[]store.Probe](t, e.do(t, "GET", "/api/probes", "", c))
	if len(list) != 1 || list[0].URL != "https://nas2.example.com/" {
		t.Fatalf("nach Umbenennen: %+v", list)
	}

	e.do(t, "PUT", "/api/records/"+itoa(rec.ID), `{"zone_id":"z1","name":"nas2.example.com","type":"A","probe":false}`, c)
	if list = decode[[]store.Probe](t, e.do(t, "GET", "/api/probes", "", c)); len(list) != 0 {
		t.Fatalf("abgeschaltet: %+v", list)
	}
}

func TestProbeSettings(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	s := decode[settingsDTO](t, e.do(t, "GET", "/api/settings", "", c))
	if s.ProbeIntervalSeconds != 300 || s.TLSWarnDays != 14 || s.Limits["tls_warn_days_max"] != 90 {
		t.Fatalf("Standard: %+v", s)
	}
	body := `{"ip_check_interval_seconds":300,"tunnel_interval_seconds":60,"notify_language":"de",
		"ip_sources":[{"name":"cloudflare","enabled":true},{"name":"ipify","enabled":true}],`
	if resp := e.do(t, "PUT", "/api/settings", body+`"tls_warn_days":200}`, c); resp.StatusCode != 400 {
		t.Fatalf("ungültig: %d", resp.StatusCode)
	}
	s = decode[settingsDTO](t, e.do(t, "PUT", "/api/settings", body+`"probe_interval_seconds":120,"tls_warn_days":21}`, c))
	if s.ProbeIntervalSeconds != 120 || s.TLSWarnDays != 21 {
		t.Fatalf("gespeichert: %+v", s)
	}
	// fehlende Felder bleiben unverändert
	s = decode[settingsDTO](t, e.do(t, "PUT", "/api/settings", strings.TrimSuffix(body, ",")+`}`, c))
	if s.ProbeIntervalSeconds != 120 || s.TLSWarnDays != 21 {
		t.Fatalf("unverändert: %+v", s)
	}
}
