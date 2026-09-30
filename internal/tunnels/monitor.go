// Package tunnels überwacht die Cloudflare Tunnels eines Accounts.
package tunnels

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

const (
	DefaultInterval = 60 * time.Second
	retention       = 30 * 24 * time.Hour
)

type lister interface {
	ListTunnels(ctx context.Context, accountID string) ([]cloudflare.Tunnel, error)
}

type tunnelStore interface {
	UpsertTunnel(ctx context.Context, t store.Tunnel) error
	MarkTunnelsRemoved(ctx context.Context, present []string, now time.Time) (int64, error)
	ListTunnels(ctx context.Context, includeRemoved bool) ([]store.Tunnel, error)
	RecordTunnelStatus(ctx context.Context, tunnelID, status string, now time.Time, maxGap time.Duration) (string, error)
	TunnelSegments(ctx context.Context, tunnelID string, since time.Time) ([]store.TunnelSegment, error)
	PurgeTunnelSegments(ctx context.Context, before time.Time) (int64, error)
}

// Change ist ein beobachteter Statuswechsel (Grundlage für Benachrichtigungen).
type Change struct {
	TunnelID string
	Name     string
	From, To string
	At       time.Time
}

type Monitor struct {
	client    lister // nil = nicht konfiguriert
	accountID string
	store     tunnelStore
	log       *slog.Logger
	pub       events.Publisher
	interval  func() time.Duration
	now       func() time.Time
	// OnChange wird für jeden Statuswechsel aufgerufen (optional).
	OnChange func(Change)

	pollMu sync.Mutex // serialisiert Abfragen
	mu     sync.Mutex
	last   *time.Time
	err    string
	purged time.Time
}

// NewMonitor erzeugt einen Monitor. client == nil oder leere accountID
// bedeutet „nicht konfiguriert“.
func NewMonitor(client lister, accountID string, st tunnelStore, log *slog.Logger, pub events.Publisher, interval func() time.Duration) *Monitor {
	if accountID == "" {
		client = nil
	}
	if interval == nil {
		interval = func() time.Duration { return DefaultInterval }
	}
	return &Monitor{client: client, accountID: accountID, store: st, log: log, pub: pub,
		interval: interval, now: time.Now}
}

func (m *Monitor) Configured() bool { return m.client != nil }

// maxGap: so lange darf eine Beobachtung zurückliegen, ohne dass eine Lücke
// entsteht (drei verpasste Abfragen, mindestens drei Minuten).
func (m *Monitor) maxGap() time.Duration { return max(3*m.interval(), 3*time.Minute) }

// Poll fragt Cloudflare ab und speichert Stand und Statusabschnitte.
func (m *Monitor) Poll(ctx context.Context) error {
	if !m.Configured() {
		return nil
	}
	m.pollMu.Lock()
	defer m.pollMu.Unlock()
	defer events.Publish(m.pub, events.TopicTunnels)

	list, err := m.client.ListTunnels(ctx, m.accountID)
	now := m.now()
	if err != nil {
		m.setResult(now, err.Error())
		return err
	}

	ids := make([]string, 0, len(list))
	for _, t := range list {
		ids = append(ids, t.ID)
		conns, _ := json.Marshal(t.Connections)
		if err := m.store.UpsertTunnel(ctx, store.Tunnel{
			ID: t.ID, Name: t.Name, Status: t.Status, CreatedAt: t.CreatedAt,
			ConnsActiveAt: t.ConnsActiveAt, ConnsInactiveAt: t.ConnsInactiveAt,
			Connections: conns, RemoteConfig: t.RemoteConfig, LastSeenAt: now,
		}); err != nil {
			return err
		}
		prev, err := m.store.RecordTunnelStatus(ctx, t.ID, t.Status, now, m.maxGap())
		if err != nil {
			return err
		}
		if prev != "" && prev != t.Status {
			m.log.Info("Tunnel-Status geändert", "tunnel", t.Name, "from", prev, "to", t.Status)
			if m.OnChange != nil {
				m.OnChange(Change{TunnelID: t.ID, Name: t.Name, From: prev, To: t.Status, At: now})
			}
		}
	}
	if n, err := m.store.MarkTunnelsRemoved(ctx, ids, now); err != nil {
		return err
	} else if n > 0 {
		m.log.Info("Tunnels nicht mehr vorhanden", "count", n)
	}

	if now.Sub(m.purged) > time.Hour {
		if _, err := m.store.PurgeTunnelSegments(ctx, now.Add(-retention)); err != nil {
			m.log.Error("Tunnel-Verlauf aufräumen fehlgeschlagen", "err", err)
		}
		m.purged = now
	}
	m.setResult(now, "")
	return nil
}

func (m *Monitor) setResult(at time.Time, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last, m.err = &at, errMsg
}

// Zeiträume der Uptime-Balken.
var ranges = []struct {
	key     string
	d       time.Duration
	buckets int
}{
	{"24h", 24 * time.Hour, 48},    // 30 min je Stück
	{"7d", 7 * 24 * time.Hour, 56}, // 3 h je Stück
}

type TunnelView struct {
	store.Tunnel
	// StatusSince: seit wann dnsdeck den aktuellen Status ununterbrochen beobachtet.
	StatusSince *time.Time        `json:"status_since,omitempty"`
	Uptime      map[string]Uptime `json:"uptime"`
}

type Overview struct {
	Configured      bool         `json:"configured"`
	LastPoll        *time.Time   `json:"last_poll,omitempty"`
	Error           string       `json:"error,omitempty"`
	IntervalSeconds int          `json:"interval_seconds"`
	Tunnels         []TunnelView `json:"tunnels"`
}

// Overview liefert alle Tunnels mit Uptime für die Anzeige.
func (m *Monitor) Overview(ctx context.Context) (Overview, error) {
	m.mu.Lock()
	o := Overview{Configured: m.Configured(), LastPoll: m.last, Error: m.err,
		IntervalSeconds: int(m.interval().Seconds()), Tunnels: []TunnelView{}}
	m.mu.Unlock()

	list, err := m.store.ListTunnels(ctx, false)
	if err != nil {
		return Overview{}, err
	}
	now := m.now()
	longest := ranges[len(ranges)-1].d
	for _, t := range list {
		segs, err := m.store.TunnelSegments(ctx, t.ID, now.Add(-longest))
		if err != nil {
			return Overview{}, err
		}
		v := TunnelView{Tunnel: t, Uptime: map[string]Uptime{},
			StatusSince: StatusSince(segs, t.Status, m.maxGap())}
		for _, r := range ranges {
			v.Uptime[r.key] = ComputeUptime(segs, now.Add(-r.d), now, r.buckets, m.interval())
		}
		o.Tunnels = append(o.Tunnels, v)
	}
	return o, nil
}
