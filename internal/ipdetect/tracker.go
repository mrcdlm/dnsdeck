package ipdetect

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
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
	IP        string        `json:"ip,omitempty"`    // zuletzt bestätigte IP
	Since     *time.Time    `json:"since,omitempty"` // seit wann diese IP gilt
	Status    string        `json:"status"`
	Message   string        `json:"message,omitempty"`
	Votes     int           `json:"votes"`
	Responses int           `json:"responses"`
	Sources   []Observation `json:"sources"`
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
	now   func() time.Time

	checkMu sync.Mutex // serialisiert Prüfungen (Scheduler + manueller Button)
	mu      sync.RWMutex
	state   State
	known   map[Family]netip.Addr
}

func NewTracker(obs observer, st changeStore, log *slog.Logger) *Tracker {
	return &Tracker{
		obs: obs, store: st, log: log, now: time.Now,
		state: State{
			IPv4: FamilyState{Status: StatusPending, Sources: []Observation{}},
			IPv6: FamilyState{Status: StatusPending, Sources: []Observation{}},
		},
		known: map[Family]netip.Addr{},
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
		fs.Status, fs.Message = StatusOK, ""
		if changed {
			t.known[f] = d.IP
			fs.IP = d.IP.String()
			since := now
			fs.Since = &since
		}
	case d.Total == 0:
		fs.Status, fs.Message = StatusUnavailable, d.Reason
	default:
		fs.Status, fs.Message = StatusUnconfirmed, d.Reason
		t.log.Warn("IP nicht bestätigt", "family", f, "reason", d.Reason)
	}
	if persistErr != nil {
		fs.Status, fs.Message = StatusUnconfirmed, "Speichern fehlgeschlagen"
	}
	return persistErr
}

// familyState liefert einen Zeiger auf den Zustand der Familie (mu gehalten).
func (t *Tracker) familyState(f Family) *FamilyState {
	if f == IPv6 {
		return &t.state.IPv6
	}
	return &t.state.IPv4
}
