package ipdetect

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/dnsbl"
	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/isp"
	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

// Status einer Adressfamilie nach der letzten Prüfung.
const (
	StatusOK          = "ok"          // per Mehrheit bestätigt
	StatusUnconfirmed = "unconfirmed" // Quellen uneinig, letzte bekannte IP bleibt
	StatusUnavailable = "unavailable" // keine Quelle erreichbar (z. B. kein IPv6)
	StatusPending     = "pending"     // noch nicht geprüft
)

type FamilyState struct {
	IP     string     `json:"ip,omitempty"`    // zuletzt bestätigte IP
	Since  *time.Time `json:"since,omitempty"` // seit wann diese IP gilt
	Status string     `json:"status"`
	// Message: von der API gerenderter Text; MessageMsg: übersetzbare Meldung.
	Message    string        `json:"message,omitempty"`
	MessageMsg i18n.Msg      `json:"-"`
	Votes      int           `json:"votes"`
	Responses  int           `json:"responses"`
	Sources    []Observation `json:"sources"`
	// ISP: Netz/Anbieter der bestätigten IP (fehlt, solange unbekannt).
	ISP *isp.Info `json:"isp,omitempty"`
	// Blocklist: Ergebnis der Sperrlisten-Prüfung (nur IPv4; fehlt, solange ungeprüft).
	Blocklist *dnsbl.Result `json:"blocklist,omitempty"`
}

type State struct {
	IPv4        FamilyState `json:"ipv4"`
	IPv6        FamilyState `json:"ipv6"`
	LastChecked *time.Time  `json:"last_checked,omitempty"`
}

type changeStore interface {
	LatestIPChange(ctx context.Context, family string) (store.IPChange, error)
	InsertIPChange(ctx context.Context, c store.IPChange) (int64, error)
}

type ispLookup interface {
	Lookup(ctx context.Context, a netip.Addr) (isp.Info, error)
}

// Wie lange ein ISP-Ergebnis gilt, bevor es erneut abgefragt wird.
const (
	ispTTL      = 24 * time.Hour
	ispRetryTTL = 15 * time.Minute
)

type blocklistChecker interface {
	Check(ctx context.Context, a netip.Addr) dnsbl.Result
}

// Wie lange ein Sperrlisten-Ergebnis gilt bzw. wann nach einer ergebnislosen
// Prüfung erneut gefragt wird.
const (
	blocklistTTL      = 24 * time.Hour
	blocklistRetryTTL = time.Hour
)

type ispEntry struct {
	ip   string
	next time.Time
}

type observer interface {
	Observe(ctx context.Context, f Family) ([]netip.Addr, []Observation)
}

// Tracker führt Prüfungen aus, protokolliert IP-Wechsel in der DB und hält den
// aktuellen Zustand für die API bereit.
type Tracker struct {
	obs   observer
	store changeStore
	log   *slog.Logger
	pub   events.Publisher
	// Notifier erhält IP-Wechsel (optional; nicht die erste Erkennung).
	Notifier notify.Notifier
	// ISP ermittelt den Anbieter der bestätigten IPs (optional).
	ISP ispLookup
	// Blocklist prüft die bestätigte IPv4 gegen DNS-Sperrlisten (optional).
	Blocklist blocklistChecker
	now       func() time.Time

	checkMu sync.Mutex // serialisiert Prüfungen (Scheduler + manueller Button)
	mu      sync.RWMutex
	state   State
	known   map[Family]netip.Addr
	isp     map[Family]ispEntry
	bl      ispEntry // nur IPv4
}

func NewTracker(obs observer, st changeStore, log *slog.Logger) *Tracker {
	return &Tracker{
		obs: obs, store: st, log: log, now: time.Now,
		state: State{
			IPv4: FamilyState{Status: StatusPending, Sources: []Observation{}},
			IPv6: FamilyState{Status: StatusPending, Sources: []Observation{}},
		},
		known: map[Family]netip.Addr{},
		isp:   map[Family]ispEntry{},
	}
}

// SetPublisher legt fest, wer über abgeschlossene Prüfungen informiert wird.
func (t *Tracker) SetPublisher(p events.Publisher) { t.pub = p }

// SetObserver tauscht die Quellen aus (z. B. nach Änderung der Settings).
func (t *Tracker) SetObserver(o observer) {
	t.checkMu.Lock()
	defer t.checkMu.Unlock()
	t.obs = o
}

// Load übernimmt die zuletzt bekannten IPs aus der DB.
func (t *Tracker) Load(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, f := range Families {
		c, err := t.store.LatestIPChange(ctx, string(f))
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		a, err := netip.ParseAddr(c.IP)
		if err != nil {
			continue
		}
		t.known[f] = a
		fs := t.familyState(f)
		fs.IP = c.IP
		since := c.DetectedAt
		fs.Since = &since
	}
	return nil
}

func (t *Tracker) State() State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s := t.state
	return s
}

// Check prüft beide Adressfamilien und liefert den neuen Zustand.
// Nur ein DB-Fehler wird als Fehler gemeldet; uneinige oder nicht erreichbare
// Quellen sind ein normaler Zustand.
func (t *Tracker) Check(ctx context.Context) (State, error) {
	t.checkMu.Lock()
	defer t.checkMu.Unlock()

	type result struct {
		addrs []netip.Addr
		obs   []Observation
	}
	results := map[Family]result{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, f := range Families {
		wg.Go(func() {
			a, o := t.obs.Observe(ctx, f)
			mu.Lock()
			results[f] = result{a, o}
			mu.Unlock()
		})
	}
	wg.Wait()

	now := t.now()
	var errs []error
	for _, f := range Families {
		if err := t.apply(ctx, f, results[f].addrs, results[f].obs, now); err != nil {
			errs = append(errs, err)
		}
	}

	// Anbieter und Sperrlisten unabhängig voneinander nachschlagen.
	var lookups sync.WaitGroup
	lookups.Go(func() { t.refreshISP(ctx, now) })
	lookups.Go(func() { t.refreshBlocklist(ctx, now) })
	lookups.Wait()

	t.mu.Lock()
	t.state.LastChecked = &now
	t.mu.Unlock()
	events.Publish(t.pub, events.TopicIP)
	return t.State(), errors.Join(errs...)
}

func (t *Tracker) apply(ctx context.Context, f Family, addrs []netip.Addr, obs []Observation, now time.Time) error {
	t.mu.RLock()
	known := t.known[f]
	t.mu.RUnlock()

	d := Decide(addrs, known)

	var persistErr error
	changed := d.Confirmed() && d.IP != known
	if changed {
		c := store.IPChange{Family: string(f), IP: d.IP.String(), DetectedAt: now}
		if known.IsValid() {
			c.PreviousIP = known.String()
		}
		if _, err := t.store.InsertIPChange(ctx, c); err != nil {
			// Nicht übernehmen, damit der Wechsel beim nächsten Lauf erneut
			// erkannt und protokolliert wird.
			persistErr = err
			changed = false
			t.log.Error("IP-Wechsel konnte nicht gespeichert werden", "family", f, "err", err)
		} else {
			t.log.Info("IP-Wechsel erkannt", "family", f, "old", c.PreviousIP, "new", c.IP)
			if c.PreviousIP != "" {
				label := map[Family]string{IPv4: "IPv4", IPv6: "IPv6"}[f]
				notify.Send(t.Notifier, notify.Event{
					Type: notify.EventIPChange, Priority: notify.PriorityDefault, Time: now,
					TitleMsg:   i18n.M("notify.ip_change.title", "family", label),
					MessageMsg: i18n.M("notify.ip_change.message", "old", c.PreviousIP, "new", c.IP),
					Data:       map[string]string{"family": string(f), "old": c.PreviousIP, "new": c.IP},
				})
			}
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	fs := t.familyState(f)
	fs.Votes, fs.Responses, fs.Sources = d.Votes, d.Total, obs
	if fs.Sources == nil {
		fs.Sources = []Observation{}
	}
	switch {
	case d.Confirmed():
		fs.Status, fs.MessageMsg = StatusOK, i18n.Msg{}
		if changed {
			t.known[f] = d.IP
			fs.IP = d.IP.String()
			since := now
			fs.Since = &since
			fs.ISP = nil // gehört zur alten IP
			fs.Blocklist = nil
		}
	case d.Total == 0:
		fs.Status, fs.MessageMsg = StatusUnavailable, d.Reason
	default:
		fs.Status, fs.MessageMsg = StatusUnconfirmed, d.Reason
		t.log.Warn("IP nicht bestätigt", "family", f, "reason", i18n.T(i18n.EN, d.Reason))
	}
	if persistErr != nil {
		fs.Status, fs.MessageMsg = StatusUnconfirmed, i18n.M("ip.save_failed")
	}
	return persistErr
}

// refreshISP ermittelt den Anbieter der bekannten IPs, sofern für die IP noch
// kein aktuelles Ergebnis vorliegt. Fehler sind nicht kritisch: Die Anzeige
// fehlt dann, und es wird später erneut versucht.
func (t *Tracker) refreshISP(ctx context.Context, now time.Time) {
	if t.ISP == nil {
		return
	}
	todo := map[Family]netip.Addr{}
	t.mu.RLock()
	for _, f := range Families {
		ip := t.familyState(f).IP
		e := t.isp[f]
		if ip == "" || (e.ip == ip && now.Before(e.next)) {
			continue
		}
		if a, err := netip.ParseAddr(ip); err == nil {
			todo[f] = a
		}
	}
	t.mu.RUnlock()

	var wg sync.WaitGroup
	for f, a := range todo {
		wg.Go(func() {
			info, err := t.ISP.Lookup(ctx, a)
			t.mu.Lock()
			defer t.mu.Unlock()
			fs := t.familyState(f)
			if fs.IP != a.String() {
				return
			}
			e := ispEntry{ip: fs.IP, next: now.Add(ispTTL)}
			if err != nil {
				e.next = now.Add(ispRetryTTL)
				if t.isp[f].ip != fs.IP {
					fs.ISP = nil
				}
				t.log.Debug("ISP nicht ermittelt", "family", f, "ip", fs.IP, "err", err)
			} else {
				if fs.ISP == nil || *fs.ISP != info {
					t.log.Info("ISP ermittelt", "family", f, "ip", fs.IP, "asn", info.ASN, "name", info.Name)
				}
				fs.ISP = &info
			}
			t.isp[f] = e
		})
	}
	wg.Wait()
}

// refreshBlocklist prüft die bekannte IPv4 gegen die Sperrlisten, sofern für
// sie noch kein aktuelles Ergebnis vorliegt. Taucht eine Listung neu auf,
// wird benachrichtigt (die PBL von Spamhaus zählt nicht als Listung).
func (t *Tracker) refreshBlocklist(ctx context.Context, now time.Time) {
	if t.Blocklist == nil {
		return
	}
	t.mu.RLock()
	ip, e := t.state.IPv4.IP, t.bl
	t.mu.RUnlock()
	if ip == "" || (e.ip == ip && now.Before(e.next)) {
		return
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return
	}

	res := t.Blocklist.Check(ctx, a)

	t.mu.Lock()
	defer t.mu.Unlock()
	fs := &t.state.IPv4
	if fs.IP != ip {
		return
	}
	prev := fs.Blocklist
	t.bl = ispEntry{ip: ip, next: now.Add(blocklistTTL)}
	if res.Status == dnsbl.StatusUnknown {
		t.bl.next = now.Add(blocklistRetryTTL)
		if prev != nil {
			// Kein Ergebnis (z. B. DNS gestört) – das letzte bleibt gültig.
			t.log.Debug("Sperrlisten nicht prüfbar, letztes Ergebnis bleibt", "ip", ip)
			return
		}
	}
	fs.Blocklist = &res

	var names, zones []string
	for _, l := range res.Listed() {
		if prev != nil && slices.ContainsFunc(prev.Listed(), func(p dnsbl.Entry) bool { return p.Zone == l.Zone }) {
			continue
		}
		names = append(names, l.Name)
		zones = append(zones, l.Zone)
	}
	if len(names) == 0 {
		return
	}
	lists := strings.Join(names, ", ")
	t.log.Warn("Öffentliche IP steht auf Sperrliste", "ip", ip, "lists", lists)
	notify.Send(t.Notifier, notify.Event{
		Type: notify.EventBlocklisted, Priority: notify.PriorityHigh, Time: now,
		TitleMsg:   i18n.M("notify.blocklist.title"),
		MessageMsg: i18n.M("notify.blocklist.message", "ip", ip, "lists", lists),
		Data:       map[string]string{"ip": ip, "lists": lists, "zones": strings.Join(zones, ",")},
	})
}

// familyState liefert einen Zeiger auf den Zustand der Familie (mu gehalten).
func (t *Tracker) familyState(f Family) *FamilyState {
	if f == IPv6 {
		return &t.state.IPv6
	}
	return &t.state.IPv4
}

// Localize liefert eine Kopie mit gerenderten Texten in lang.
func (s State) Localize(lang i18n.Lang) State {
	loc := func(f FamilyState) FamilyState {
		f.Message = i18n.T(lang, f.MessageMsg)
		src := make([]Observation, len(f.Sources))
		for i, o := range f.Sources {
			o.Error = i18n.T(lang, o.ErrorMsg)
			src[i] = o
		}
		f.Sources = src
		return f
	}
	s.IPv4, s.IPv6 = loc(s.IPv4), loc(s.IPv6)
	return s
}
