package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

type webhookStore interface {
	ListWebhooks(ctx context.Context) ([]store.Webhook, error)
	GetWebhook(ctx context.Context, id int64) (store.Webhook, error)
	SetWebhookResult(ctx context.Context, id int64, at time.Time, err error) error
}

// Dispatcher stellt Ereignisse asynchron an alle aktiven Webhooks zu, die das
// Ereignis abonniert haben. Die Konfiguration wird je Ereignis frisch gelesen.
// Eine volle Warteschlange verwirft Ereignisse, statt die Überwachung zu
// blockieren.
type Dispatcher struct {
	// Pub wird nach Zustellungen informiert, damit die UI den Status live zeigt.
	Pub events.Publisher
	// Lang liefert die Sprache der Benachrichtigungen (nil = Deutsch).
	Lang    func() i18n.Lang
	store   webhookStore
	env     Env
	log     *slog.Logger
	client  *http.Client
	queue   chan Event
	retries int
	backoff time.Duration
	wg      sync.WaitGroup
}

func NewDispatcher(st webhookStore, env Env, log *slog.Logger) *Dispatcher {
	return &Dispatcher{store: st, env: env, log: log,
		client: &http.Client{
			Timeout: 15 * time.Second,
			// Weiterleitungen nur auf denselben Host: Go entfernt bei fremden
			// Hosts nur Authorization/Cookie, nicht z. B. X-Gotify-Key.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 || req.URL.Host != via[0].URL.Host {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		queue:   make(chan Event, 100),
		retries: 3, backoff: 5 * time.Second}
}

// Start verarbeitet die Warteschlange, bis ctx endet.
func (d *Dispatcher) Start(ctx context.Context) {
	d.wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-d.queue:
				d.dispatch(ctx, ev)
			}
		}
	})
}

// Wait wartet auf das Ende der Verarbeitung (nach Abbruch von ctx).
func (d *Dispatcher) Wait() { d.wg.Wait() }

func (d *Dispatcher) Notify(ev Event) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	if ev.Priority == 0 {
		ev.Priority = PriorityDefault
	}
	ev = Localize(ev, d.lang())
	select {
	case d.queue <- ev:
	default:
		d.log.Warn("Benachrichtigung verworfen – Warteschlange voll", "type", ev.Type)
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, ev Event) {
	hooks, err := d.store.ListWebhooks(ctx)
	if err != nil {
		d.log.Error("Webhooks lesen fehlgeschlagen", "err", err)
		return
	}
	// Parallel zustellen: ein hängender Webhook darf die anderen nicht aufhalten.
	var wg sync.WaitGroup
	delivered := false
	for _, w := range hooks {
		if !w.Enabled || !slices.Contains(w.Events, ev.Type) {
			continue
		}
		delivered = true
		wg.Go(func() {
			err := d.deliver(ctx, w, ev, d.retries)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				d.log.Warn("Webhook fehlgeschlagen", "webhook", w.Name, "type", ev.Type, "err", err)
			}
			if err := d.store.SetWebhookResult(ctx, w.ID, time.Now(), err); err != nil {
				d.log.Error("Webhook-Status speichern fehlgeschlagen", "err", err)
			}
		})
	}
	wg.Wait()
	if delivered {
		events.Publish(d.Pub, events.TopicWebhooks)
	}
}

// deliver rendert und sendet mit bis zu attempts Versuchen. Konfigurations-
// fehler (z. B. fehlende Env-Variable) werden nicht wiederholt.
func (d *Dispatcher) deliver(ctx context.Context, w store.Webhook, ev Event, attempts int) error {
	req, err := Render(w, ev, d.env)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.backoff * time.Duration(attempt)):
			}
		}
		if err = d.send(ctx, req); err == nil {
			return nil
		}
	}
	return err
}

// send schickt die Anfrage. Fehlermeldungen enthalten nie die URL, weil sie
// aufgelöste Geheimnisse enthalten kann.
func (d *Dispatcher) send(ctx context.Context, r Request) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var body io.Reader
	if r.Body != "" {
		body = strings.NewReader(r.Body)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, body)
	if err != nil {
		return i18n.E("webhook.bad_url")
	}
	for k, v := range r.Headers {
		req.Header[k] = v
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "dnsdeck")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return i18n.Wrap(err, "webhook.unreachable")
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return i18n.E("webhook.redirect", "status", strconv.Itoa(resp.StatusCode))
	}
	if resp.StatusCode >= 400 {
		return i18n.E("webhook.http", "status", strconv.Itoa(resp.StatusCode))
	}
	return nil
}

func (d *Dispatcher) lang() i18n.Lang {
	if d.Lang == nil {
		return i18n.DE
	}
	return d.Lang()
}

// Preview rendert eine Konfiguration mit Beispielereignis in der
// Benachrichtigungssprache – ohne Geheimnisse (Platzhalter bleiben).
func (d *Dispatcher) Preview(w store.Webhook, eventType string) (Request, error) {
	return Render(w, Localize(SampleEvent(eventType), d.lang()), nil)
}

// TestResult ist das Ergebnis einer Testnachricht.
type TestResult struct {
	OK    bool     `json:"ok"`
	Error i18n.Msg `json:"-"`
}

// SendTest schickt synchron eine Testnachricht (ein Versuch) über einen Webhook,
// auch wenn er deaktiviert ist oder das Test-Ereignis nicht abonniert hat.
func (d *Dispatcher) SendTest(ctx context.Context, id int64) (TestResult, error) {
	w, err := d.store.GetWebhook(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	sendErr := d.deliver(ctx, w, Localize(SampleEvent(EventTest), d.lang()), 1)
	if err := d.store.SetWebhookResult(ctx, w.ID, time.Now(), sendErr); err != nil {
		return TestResult{}, err
	}
	if sendErr != nil {
		return TestResult{Error: i18n.FromError(sendErr)}, nil
	}
	return TestResult{OK: true}, nil
}
