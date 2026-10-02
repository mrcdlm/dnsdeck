package probe

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func up(certDays int) Result {
	return Result{HTTPStatus: 200, Latency: 42 * time.Millisecond,
		TLS: &TLSInfo{NotAfter: t0.Add(time.Duration(certDays) * 24 * time.Hour), Issuer: "Test CA"}}
}

func down() Result { return Result{Latency: time.Second, Err: i18n.M("probe.timeout")} }

func types(evs []notify.Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Type)
	}
	return out
}

// apply übernimmt ein Ergebnis in die Prüfung, wie es der Store täte.
func apply(p store.Probe, r store.ProbeResult) store.Probe {
	p.Status, p.FailCount, p.CertWarnedFor = r.Status, r.FailCount, r.CertWarnedFor
	p.TLSNotAfter, p.TLSIssuer, p.TLSValid = r.TLSNotAfter, r.TLSIssuer, r.TLSValid
	return p
}

func TestEvaluateDebounceAndEvents(t *testing.T) {
	p := store.Probe{ID: 1, URL: "https://nas.example.com/", Enabled: true, Status: store.ProbePending}

	// erste Prüfung: erreichbar, keine Meldung
	r, evs := Evaluate(p, up(60), t0, 14)
	if r.Status != store.ProbeUp || !r.Changed || len(evs) != 0 || *r.LatencyMS != 42 || !*r.TLSValid {
		t.Fatalf("erste Prüfung: %+v %v", r, types(evs))
	}
	p = apply(p, r)

	// ein Fehlschlag → Status bleibt, Hinweis auf Wiederholung, Zertifikat bleibt bekannt
	r, evs = Evaluate(p, down(), t0, 14)
	if r.Status != store.ProbeUp || r.Changed || r.FailCount != 1 || len(evs) != 0 ||
		r.Message.Code != "probe.retrying" || r.TLSNotAfter == nil || r.LatencyMS != nil {
		t.Fatalf("1. Fehlschlag: %+v %v", r, types(evs))
	}
	p = apply(p, r)

	// zweiter Fehlschlag → nicht erreichbar + Meldung
	r, evs = Evaluate(p, down(), t0, 14)
	if r.Status != store.ProbeDown || !r.Changed || r.FailCount != 2 || len(evs) != 1 ||
		evs[0].Type != notify.EventSiteDown || evs[0].Data["host"] != "nas.example.com" || r.Message.Code != "probe.timeout" {
		t.Fatalf("2. Fehlschlag: %+v %v", r, types(evs))
	}
	p = apply(p, r)

	// weiterhin down → keine erneute Meldung
	if r, evs = Evaluate(p, down(), t0, 14); r.Changed || len(evs) != 0 {
		t.Fatalf("weiter down: %+v %v", r, types(evs))
	}
	p = apply(p, r)

	// wieder erreichbar → Meldung, Zähler zurück
	r, evs = Evaluate(p, up(60), t0, 14)
	if r.Status != store.ProbeUp || r.FailCount != 0 || len(evs) != 1 || evs[0].Type != notify.EventSiteRecovered {
		t.Fatalf("wieder da: %+v %v", r, types(evs))
	}
}

func TestEvaluateFirstCheckDownImmediately(t *testing.T) {
	p := store.Probe{URL: "https://x.example/", Enabled: true, Status: store.ProbePending}
	if r, evs := Evaluate(p, down(), t0, 14); r.Status != store.ProbeDown || len(evs) != 0 {
		t.Fatalf("%+v %v", r, types(evs))
	}
}

func TestEvaluateCertificate(t *testing.T) {
	p := store.Probe{URL: "https://nas.example.com/", Enabled: true, Status: store.ProbeUp}

	// bald ablaufend → Status + einmalige Warnung
	r, evs := Evaluate(p, up(10), t0, 14)
	if r.Status != store.ProbeExpiring || r.Message.Code != "probe.cert_expires" || r.Message.Params["days"] != "10" ||
		len(evs) != 1 || evs[0].Type != notify.EventCertExpiring || evs[0].Data["days"] != "10" || r.CertWarnedFor == nil {
		t.Fatalf("Warnung: %+v %v", r, types(evs))
	}
	p = apply(p, r)
	// dasselbe Zertifikat einen Tag später → keine zweite Warnung
	if r, evs = Evaluate(p, up(10), t0.Add(24*time.Hour), 14); len(evs) != 0 || r.Status != store.ProbeExpiring ||
		r.Message.Params["days"] != "9" {
		t.Fatalf("zweite Warnung: %v", types(evs))
	}
	p = apply(p, r)

	// erneuertes Zertifikat → wieder in Ordnung; läuft es später ab, wird erneut gewarnt
	r, evs = Evaluate(p, up(90), t0, 14)
	if r.Status != store.ProbeUp || len(evs) != 0 {
		t.Fatalf("erneuert: %+v %v", r, types(evs))
	}
	p = apply(p, r)
	if _, evs = Evaluate(p, up(90), t0.Add(80*24*time.Hour), 14); len(evs) != 1 {
		t.Fatalf("neues Zertifikat läuft ab: %v", types(evs))
	}

	// ungültiges Zertifikat → Problem, auch ohne Entprellung
	bad := up(60)
	bad.TLS.Err = i18n.M("probe.cert_hostname", "host", "nas.example.com")
	r, evs = Evaluate(store.Probe{URL: p.URL, Status: store.ProbeUp}, bad, t0, 14)
	if r.Status != store.ProbeTLSError || *r.TLSValid || len(evs) != 1 || evs[0].Type != notify.EventSiteDown {
		t.Fatalf("ungültig: %+v %v", r, types(evs))
	}
}

type fakeChecker struct {
	mu      sync.Mutex
	results map[string]Result
	calls   map[string]int
}

func (f *fakeChecker) Check(_ context.Context, url string) Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[url]++
	return f.results[url]
}

type eventLog struct {
	mu  sync.Mutex
	evs []notify.Event
}

func (l *eventLog) Notify(ev notify.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evs = append(l.evs, ev)
}

func TestMonitorPoll(t *testing.T) {
	ctx := t.Context()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a, _ := st.CreateProbe(ctx, "https://a.example/", true)
	b, _ := st.CreateProbe(ctx, "https://b.example/", true)
	off, _ := st.CreateProbe(ctx, "https://off.example/", false)

	fc := &fakeChecker{calls: map[string]int{}, results: map[string]Result{
		a.URL: up(60), b.URL: up(5), off.URL: up(60)}}
	var log eventLog
	m := NewMonitor(st, fc, slog.New(slog.DiscardHandler), nil)
	m.Notifier, m.now = &log, func() time.Time { return t0 }
	m.WarnDays = func() int { return 7 }

	if err := m.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	if fc.calls[off.URL] != 0 || fc.calls[a.URL] != 1 {
		t.Fatalf("Abfragen: %v", fc.calls)
	}
	ga, _ := st.GetProbe(ctx, a.ID)
	gb, _ := st.GetProbe(ctx, b.ID)
	if ga.Status != store.ProbeUp || *ga.HTTPStatus != 200 || ga.TLSIssuer != "Test CA" || ga.LastCheckedAt == nil ||
		gb.Status != store.ProbeExpiring || gb.CertWarnedFor == nil || len(log.evs) != 1 {
		t.Fatalf("nach Poll: %+v / %+v / %d Ereignisse", ga, gb, len(log.evs))
	}

	// Ausfall über zwei Läufe (einer davon manuell)
	fc.results[a.URL] = down()
	m.Poll(ctx)
	if g, _ := st.GetProbe(ctx, a.ID); g.Status != store.ProbeUp || g.FailCount != 1 {
		t.Fatalf("1. Fehlschlag: %+v", g)
	}
	g, err := m.Run(ctx, a.ID)
	if err != nil || g.Status != store.ProbeDown || g.MessageMsg.Code != "probe.timeout" || len(log.evs) != 2 {
		t.Fatalf("2. Fehlschlag: %+v %v / %d Ereignisse", g, err, len(log.evs))
	}

	// Während der Prüfung geänderte Adresse → altes Ergebnis wird verworfen
	if _, err := st.UpdateProbe(ctx, a.ID, "https://neu.example/", true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetProbeResult(ctx, a.ID, a.URL, store.ProbeResult{Status: store.ProbeUp, CheckedAt: t0}); err != nil {
		t.Fatal(err)
	}
	if g, _ := st.GetProbe(ctx, a.ID); g.Status != store.ProbePending || g.FailCount != 0 || g.HTTPStatus != nil {
		t.Fatalf("nach Adressänderung: %+v", g)
	}
}
