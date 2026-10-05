package probe

import (
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

func TestParseExpected(t *testing.T) {
	good := map[string]string{
		"":             "",
		" 200 ":        "200",
		"200, 204":     "200,204",
		"2XX":          "2xx",
		"200-299, 301": "200-299,301",
		"200 - 399":    "200 - 399",
	}
	for in, want := range good {
		if _, spec, err := ParseExpected(in); err != nil || spec != want {
			t.Errorf("%q → %q, %v (erwartet %q)", in, spec, err, want)
		}
	}
	for _, bad := range []string{"99", "600", "abc", "300-200", "6xx", "2x", "200-"} {
		if _, _, err := ParseExpected(bad); err == nil {
			t.Errorf("%q: Fehler erwartet", bad)
		}
	}
	e, _, _ := ParseExpected("2xx,301,400-401")
	for code, want := range map[int]bool{200: true, 299: true, 301: true, 302: false, 400: true, 401: true, 404: false, 503: false} {
		if e.Match(code) != want {
			t.Errorf("Match(%d) = %v", code, !want)
		}
	}
}

func TestEvaluateExpectedStatus(t *testing.T) {
	ok := Result{HTTPStatus: 200}
	p := store.Probe{URL: "https://x.example/", Status: store.ProbePending, ExpectedStatus: "204"}
	r, _ := Evaluate(p, ok, t0, 14)
	if r.Status != store.ProbeDown || r.Message.Code != "probe.status_unexpected" ||
		r.Message.Params["status"] != "200" || r.Message.Params["expected"] != "204" {
		t.Fatalf("unerwartet: %+v", r)
	}

	// Erwartetes 503 (z. B. Wartungsseite) gilt als erreichbar.
	p.ExpectedStatus = "200,503"
	maint := Result{HTTPStatus: 503, Err: down().Err}
	if r, _ = Evaluate(p, maint, t0, 14); r.Status != store.ProbeUp {
		t.Fatalf("erwartetes 503: %+v", r)
	}
	// Ohne Antwort hilft keine Erwartung.
	if r, _ = Evaluate(p, down(), t0, 14); r.Status != store.ProbeDown || r.Message.Code != "probe.timeout" {
		t.Fatalf("keine Antwort: %+v", r)
	}
	// Auch ein unerwarteter Status wird entprellt.
	p.Status = store.ProbeUp
	if r, _ = Evaluate(p, Result{HTTPStatus: 404}, t0, 14); r.Status != store.ProbeUp || r.Message.Code != "probe.retrying" {
		t.Fatalf("Entprellung: %+v", r)
	}
}

func TestUptimes(t *testing.T) {
	now := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	segs := []store.Segment{
		{Status: store.ProbeUp, StartedAt: now.Add(-24 * time.Hour), LastSeenAt: now.Add(-12 * time.Hour)},
		{Status: store.ProbeDown, StartedAt: now.Add(-12 * time.Hour), LastSeenAt: now.Add(-6 * time.Hour)},
		{Status: store.ProbeExpiring, StartedAt: now.Add(-6 * time.Hour), LastSeenAt: now},
	}
	u := Uptimes(segs, now, 5*time.Minute)
	day := u["24h"]
	if day.Percent == nil || *day.Percent < 74 || *day.Percent > 76 {
		t.Fatalf("24h: %+v", day.Percent)
	}
	if day.Buckets[0] != "healthy" || day.Buckets[30] != "down" || day.Buckets[47] != "degraded" {
		t.Fatalf("Balken: %v", day.Buckets)
	}
	if _, ok := u["7d"]; !ok {
		t.Fatal("7d fehlt")
	}
}
