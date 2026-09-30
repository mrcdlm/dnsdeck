// Package notify verschickt Benachrichtigungen über Webhook, ntfy und Gotify.
//
// Zugangsdaten (URLs, Tokens) kommen ausschließlich aus Env-Variablen und
// werden weder gespeichert noch geloggt noch über die API ausgeliefert – die
// API zeigt nur Typ und Host (siehe Channel.Target).
package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Ereignistypen, die einzeln an- und abgeschaltet werden können.
const (
	EventIPChange        = "ip_change"
	EventUpdateFailed    = "update_failed"
	EventUpdateRecovered = "update_recovered"
	EventTunnelStatus    = "tunnel_status"
	EventTest            = "test" // Testnachricht, immer zugestellt
)

// EventTypes in Anzeigereihenfolge.
var EventTypes = []string{EventIPChange, EventUpdateFailed, EventUpdateRecovered, EventTunnelStatus}

// Priorität (angelehnt an ntfy: 1 min … 5 max).
const (
	PriorityLow     = 2
	PriorityDefault = 3
	PriorityHigh    = 4
)

type Event struct {
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Message  string            `json:"message"`
	Priority int               `json:"priority"`
	Time     time.Time         `json:"time"`
	Data     map[string]string `json:"data,omitempty"`
}

// Notifier nimmt Ereignisse entgegen (nil-sicher über Send).
type Notifier interface {
	Notify(ev Event)
}

// Send leitet ev an n weiter, wenn n gesetzt ist.
func Send(n Notifier, ev Event) {
	if n != nil {
		n.Notify(ev)
	}
}

// Channel ist ein Zustellweg.
type Channel interface {
	// Type: "webhook" | "ntfy" | "gotify"
	Type() string
	// Target: Anzeige ohne Geheimnisse, z. B. "https://ntfy.sh/…".
	Target() string
	Send(ctx context.Context, ev Event) error
}

// ChannelStatus ist das Ergebnis der letzten Zustellung über einen Kanal.
type ChannelStatus struct {
	Type     string     `json:"type"`
	Target   string     `json:"target"`
	LastSent *time.Time `json:"last_sent,omitempty"`
	LastErr  string     `json:"last_error,omitempty"`
}

// Dispatcher stellt Ereignisse asynchron über alle Kanäle zu, deren Typ
// aktiviert ist. Eine volle Warteschlange verwirft Ereignisse, statt die
// Überwachung zu blockieren.
type Dispatcher struct {
	channels []Channel
	enabled  func(eventType string) bool
	log      *slog.Logger
	queue    chan Event
	retries  int
	backoff  time.Duration

	mu     sync.Mutex
	status []ChannelStatus
	wg     sync.WaitGroup
}

// NewDispatcher erzeugt einen Dispatcher; enabled entscheidet je Ereignistyp.
func NewDispatcher(channels []Channel, enabled func(string) bool, log *slog.Logger) *Dispatcher {
	d := &Dispatcher{channels: channels, enabled: enabled, log: log,
		queue: make(chan Event, 100), retries: 3, backoff: 5 * time.Second}
	for _, c := range channels {
		d.status = append(d.status, ChannelStatus{Type: c.Type(), Target: c.Target()})
	}
	return d
}

// Start verarbeitet die Warteschlange, bis ctx endet.
func (d *Dispatcher) Start(ctx context.Context) {
	d.wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-d.queue:
				for i := range d.channels {
					d.deliver(ctx, i, ev)
				}
			}
		}
	})
}

// Wait wartet auf das Ende der Verarbeitung (nach Abbruch von ctx).
func (d *Dispatcher) Wait() { d.wg.Wait() }

func (d *Dispatcher) Notify(ev Event) {
	if len(d.channels) == 0 || (ev.Type != EventTest && d.enabled != nil && !d.enabled(ev.Type)) {
		return
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	if ev.Priority == 0 {
		ev.Priority = PriorityDefault
	}
	select {
	case d.queue <- ev:
	default:
		d.log.Warn("Benachrichtigung verworfen – Warteschlange voll", "type", ev.Type)
	}
}

func (d *Dispatcher) deliver(ctx context.Context, i int, ev Event) {
	c := d.channels[i]
	var err error
	for attempt := 0; attempt < d.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d.backoff * time.Duration(attempt)):
			}
		}
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = c.Send(sendCtx, ev)
		cancel()
		if err == nil {
			break
		}
	}
	d.record(i, err)
	if err != nil {
		d.log.Warn("Benachrichtigung fehlgeschlagen", "channel", c.Type(), "type", ev.Type, "err", err)
	}
}

func (d *Dispatcher) record(i int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		d.status[i].LastErr = err.Error()
		return
	}
	now := time.Now()
	d.status[i].LastSent, d.status[i].LastErr = &now, ""
}

// Status liefert den Zustand aller Kanäle.
func (d *Dispatcher) Status() []ChannelStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]ChannelStatus{}, d.status...)
}

// TestResult ist das Ergebnis einer Testnachricht je Kanal.
type TestResult struct {
	Type   string `json:"type"`
	Target string `json:"target"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// SendTest schickt synchron (ohne Warteschlange, ohne Wiederholung) eine
// Testnachricht über alle Kanäle.
func (d *Dispatcher) SendTest(ctx context.Context) []TestResult {
	ev := Event{Type: EventTest, Title: "dnsdeck: Testnachricht",
		Message: "Benachrichtigungen funktionieren.", Priority: PriorityDefault, Time: time.Now()}
	out := make([]TestResult, len(d.channels))
	var wg sync.WaitGroup
	for i, c := range d.channels {
		wg.Go(func() {
			sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			err := c.Send(sendCtx, ev)
			d.record(i, err)
			out[i] = TestResult{Type: c.Type(), Target: c.Target(), OK: err == nil}
			if err != nil {
				out[i].Error = err.Error()
			}
		})
	}
	wg.Wait()
	return out
}
