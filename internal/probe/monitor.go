package probe

import (
	"context"
	"log/slog"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

const (
	DefaultWarnDays = 14
	// FailThreshold: so viele Fehlschläge in Folge, bevor ein erreichbarer
	// Dienst als nicht erreichbar gilt (gegen kurze Aussetzer).
	FailThreshold = 2
	concurrency   = 4
)

type probeStore interface {
	ListProbes(ctx context.Context) ([]store.Probe, error)
	GetProbe(ctx context.Context, id int64) (store.Probe, error)
	SetProbeResult(ctx context.Context, id int64, url string, r store.ProbeResult) error
	RecordProbeStatus(ctx context.Context, probeID int64, status string, now time.Time, maxGap time.Duration) error
	PurgeProbeSegments(ctx context.Context, before time.Time) (int64, error)
}

// Aufbewahrung des Statusverlaufs (wie bei Tunnels).
const retention = 30 * 24 * time.Hour

type checker interface {
	Check(ctx context.Context, rawURL string) Result
}

// Monitor prüft alle aktiven Prüfungen, speichert die Ergebnisse und meldet
// Statuswechsel und ablaufende Zertifikate.
type Monitor struct {
	store   probeStore
	checker checker
	log     *slog.Logger
	pub     events.Publisher
	// WarnDays: ab wie vielen Tagen Restlaufzeit vor dem Zertifikat gewarnt wird.
	WarnDays func() int
	// Interval: Abstand der geplanten Prüfungen (für den Verlauf: so lange
	// gilt eine Beobachtung, Lücken darüber hinaus bleiben unbekannt).
	Interval func() time.Duration
	// Notifier erhält Ereignisse (optional).
	Notifier notify.Notifier
	now      func() time.Time

	mu     sync.Mutex
	locks  map[int64]*sync.Mutex // je Prüfung, damit Zeitplan und Button sich nicht überholen
	purged time.Time
}

func NewMonitor(st probeStore, c checker, log *slog.Logger, pub events.Publisher) *Monitor {
	return &Monitor{store: st, checker: c, log: log, pub: pub, now: time.Now,
		WarnDays: func() int { return DefaultWarnDays },
		Interval: func() time.Duration { return DefaultInterval },
		locks:    map[int64]*sync.Mutex{}}
}

// DefaultInterval entspricht config.DefaultProbeInterval.
const DefaultInterval = 5 * time.Minute

// maxGap: so lange darf die letzte Prüfung zurückliegen, ohne dass der Verlauf
// eine Lücke bekommt (drei verpasste Prüfungen, mindestens 15 Minuten).
func (m *Monitor) maxGap() time.Duration { return max(3*m.Interval(), 15*time.Minute) }

// Poll prüft alle aktiven Prüfungen (begrenzt parallel).
func (m *Monitor) Poll(ctx context.Context) error {
	probes, err := m.store.ListProbes(ctx)
	if err != nil {
		return err
	}
	defer events.Publish(m.pub, events.TopicProbes)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, p := range probes {
		if !p.Enabled {
			continue
		}
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := m.run(ctx, p.ID); err != nil && ctx.Err() == nil {
				m.log.Error("Erreichbarkeit prüfen fehlgeschlagen", "url", p.URL, "err", err)
			}
		})
	}
	wg.Wait()

	m.mu.Lock()
	purge := m.now().Sub(m.purged) > time.Hour
	if purge {
		m.purged = m.now()
	}
	m.mu.Unlock()
	if purge {
		if _, err := m.store.PurgeProbeSegments(ctx, m.now().Add(-retention)); err != nil {
			m.log.Error("Verlauf der Prüfungen aufräumen fehlgeschlagen", "err", err)
		}
	}
	return nil
}

// Run prüft eine einzelne Prüfung sofort (deaktivierte werden übersprungen).
func (m *Monitor) Run(ctx context.Context, id int64) (store.Probe, error) {
	if err := m.run(ctx, id); err != nil {
		return store.Probe{}, err
	}
	events.Publish(m.pub, events.TopicProbes)
	return m.store.GetProbe(ctx, id)
}

func (m *Monitor) lock(id int64) func() {
	m.mu.Lock()
	l, ok := m.locks[id]
	if !ok {
		l = &sync.Mutex{}
		m.locks[id] = l
	}
	m.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (m *Monitor) run(ctx context.Context, id int64) error {
	defer m.lock(id)()
	// Erst nach dem Sperren lesen: Fehlerzähler und Status sind dann aktuell.
	p, err := m.store.GetProbe(ctx, id)
	if err != nil || !p.Enabled {
		return err
	}
	res := m.checker.Check(ctx, p.URL)
	if ctx.Err() != nil {
		return ctx.Err() // abgebrochen (Shutdown) – kein Ergebnis speichern
	}
	now := m.now()
	r, evs := Evaluate(p, res, now, m.WarnDays())
	if err := m.store.SetProbeResult(ctx, p.ID, p.URL, r); err != nil {
		return err
	}
	if err := m.store.RecordProbeStatus(ctx, p.ID, r.Status, now, m.maxGap()); err != nil {
		m.log.Warn("Verlauf der Prüfung nicht gespeichert", "url", p.URL, "err", err)
	}
	if r.Changed {
		m.log.Info("Erreichbarkeit geändert", "url", p.URL, "from", p.Status, "to", r.Status,
			"detail", i18n.T(i18n.EN, r.Message))
	}
	for _, ev := range evs {
		notify.Send(m.Notifier, ev)
	}
	return nil
}

func problem(status string) bool { return status == store.ProbeDown || status == store.ProbeTLSError }
func healthy(status string) bool { return status == store.ProbeUp || status == store.ProbeExpiring }

// Evaluate bewertet ein Prüfergebnis gegenüber dem bisherigen Stand und
// liefert den neuen Stand sowie die fälligen Benachrichtigungen.
func Evaluate(p store.Probe, res Result, now time.Time, warnDays int) (store.ProbeResult, []notify.Event) {
	r := store.ProbeResult{CheckedAt: now, CertWarnedFor: p.CertWarnedFor}
	if res.HTTPStatus > 0 {
		r.HTTPStatus = &res.HTTPStatus
	}
	// Mit erwarteten Statuscodes entscheidet allein der Code – auch ein
	// erwartetes 503 (Wartungsseite) gilt dann als erreichbar.
	failure := res.Err
	if exp, spec, err := ParseExpected(p.ExpectedStatus); err == nil && len(exp) > 0 && res.HTTPStatus > 0 {
		failure = i18n.Msg{}
		if !exp.Match(res.HTTPStatus) {
			failure = i18n.M("probe.status_unexpected", "status", strconv.Itoa(res.HTTPStatus), "expected", spec)
		}
	}
	up := failure.IsZero()
	if ms := int(res.Latency.Milliseconds()); up {
		r.LatencyMS = &ms
	}
	host := hostOf(p.URL)

	// Zertifikat: neu gelesen, oder bei Verbindungsfehlern das zuletzt bekannte.
	switch {
	case res.TLS != nil:
		na, valid := res.TLS.NotAfter, res.TLS.Err.IsZero()
		r.TLSNotAfter, r.TLSIssuer, r.TLSValid = &na, res.TLS.Issuer, &valid
	case !up:
		r.TLSNotAfter, r.TLSIssuer, r.TLSValid = p.TLSNotAfter, p.TLSIssuer, p.TLSValid
	}

	var days int
	switch {
	case !up:
		r.Status, r.Message = store.ProbeDown, failure
	case res.TLS != nil && !res.TLS.Err.IsZero():
		r.Status, r.Message = store.ProbeTLSError, res.TLS.Err
	case res.TLS != nil && res.TLS.NotAfter.Sub(now) <= time.Duration(warnDays)*24*time.Hour:
		days = int(res.TLS.NotAfter.Sub(now).Hours() / 24)
		r.Status = store.ProbeExpiring
		r.Message = i18n.M("probe.cert_expires", "date", res.TLS.NotAfter.Format(time.DateOnly), "days", strconv.Itoa(days))
	default:
		r.Status = store.ProbeUp
	}

	// Entprellung: Ein einzelner Fehlschlag ändert einen bekannten Status noch nicht.
	if r.Status == store.ProbeDown {
		r.FailCount = p.FailCount + 1
		if r.FailCount < FailThreshold && p.Status != store.ProbePending && p.Status != store.ProbeDown {
			r.Status = p.Status
			r.Message = i18n.M("probe.retrying", "detail", i18n.Nest(failure))
		}
	}
	r.Changed = r.Status != p.Status

	var evs []notify.Event
	data := map[string]string{"url": p.URL, "host": host, "status": r.Status}
	if r.HTTPStatus != nil {
		data["http_status"] = strconv.Itoa(*r.HTTPStatus)
	}
	switch {
	case healthy(p.Status) && problem(r.Status):
		data["error"] = i18n.T(i18n.EN, r.Message)
		evs = append(evs, notify.Event{Type: notify.EventSiteDown, Priority: notify.PriorityHigh, Time: now,
			TitleMsg: i18n.M("notify.site_down.title", "host", host), MessageMsg: r.Message, Data: data})
	case problem(p.Status) && healthy(r.Status):
		evs = append(evs, notify.Event{Type: notify.EventSiteRecovered, Priority: notify.PriorityDefault, Time: now,
			TitleMsg:   i18n.M("notify.site_recovered.title", "host", host),
			MessageMsg: i18n.M("notify.site_recovered.message", "url", p.URL), Data: data})
	}

	// Vor jedem Zertifikat nur einmal warnen; ein neues Zertifikat setzt das zurück.
	if r.Status == store.ProbeExpiring && r.TLSNotAfter != nil && (p.CertWarnedFor == nil || !p.CertWarnedFor.Equal(*r.TLSNotAfter)) {
		na := *r.TLSNotAfter
		r.CertWarnedFor = &na
		evs = append(evs, notify.Event{Type: notify.EventCertExpiring, Priority: notify.PriorityDefault, Time: now,
			TitleMsg:   i18n.M("notify.cert_expiring.title", "host", host),
			MessageMsg: i18n.M("probe.cert_expires", "date", na.Format(time.DateOnly), "days", strconv.Itoa(days)),
			Data: map[string]string{"url": p.URL, "host": host, "not_after": na.Format(time.RFC3339),
				"days": strconv.Itoa(days), "issuer": r.TLSIssuer}})
	}
	return r, evs
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return raw
}
