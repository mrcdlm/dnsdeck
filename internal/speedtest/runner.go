package speedtest

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

const (
	// Retention: so lange bleiben Messungen gespeichert.
	Retention = 365 * 24 * time.Hour
	// maxRun begrenzt eine Messung (normal ~20 s).
	maxRun = 2 * time.Minute
	// idleRecheck: Abstand, in dem ein abgeschalteter Zeitplan neu geprüft wird.
	idleRecheck = time.Hour
	minDelay    = time.Minute
)

type measurer interface {
	Run(ctx context.Context) (Result, error)
}

type runnerStore interface {
	InsertSpeedtest(ctx context.Context, t store.Speedtest) (int64, error)
	LastSpeedtest(ctx context.Context) (store.Speedtest, error)
	PruneSpeedtests(ctx context.Context, before time.Time) (int64, error)
}

// Runner führt Messungen aus (per Button oder nach Zeitplan), speichert sie
// und verhindert, dass zwei Messungen gleichzeitig laufen – sie würden sich
// gegenseitig die Bandbreite wegnehmen.
type Runner struct {
	base   context.Context // endet beim Herunterfahren
	client measurer
	store  runnerStore
	log    *slog.Logger
	pub    events.Publisher
	// Interval: Abstand geplanter Messungen; 0 = nur manuell.
	Interval func() time.Duration
	now      func() time.Time

	mu        sync.Mutex
	running   bool
	startedAt time.Time
	wg        sync.WaitGroup
}

func NewRunner(ctx context.Context, c measurer, st runnerStore, log *slog.Logger, pub events.Publisher) *Runner {
	return &Runner{base: ctx, client: c, store: st, log: log, pub: pub, now: time.Now,
		Interval: func() time.Duration { return 0 }}
}

// Status meldet, ob gerade gemessen wird, und seit wann.
func (r *Runner) Status() (running bool, since time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running, r.startedAt
}

func (r *Runner) begin() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return false
	}
	r.running, r.startedAt = true, r.now()
	return true
}

func (r *Runner) end() {
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
	events.Publish(r.pub, events.TopicSpeedtest)
}

// Start beginnt eine Messung im Hintergrund. false: es läuft bereits eine.
func (r *Runner) Start(trigger string) bool {
	if !r.begin() {
		return false
	}
	events.Publish(r.pub, events.TopicSpeedtest)
	r.wg.Go(func() {
		defer r.end()
		if err := r.measure(r.base, trigger); err != nil && r.base.Err() == nil {
			r.log.Error("Speedtest speichern fehlgeschlagen", "err", err)
		}
	})
	return true
}

// Wait wartet auf eine laufende Hintergrundmessung (beim Herunterfahren).
func (r *Runner) Wait() { r.wg.Wait() }

// Poll misst, wenn laut Zeitplan eine Messung fällig ist (für den Scheduler).
func (r *Runner) Poll(ctx context.Context) error {
	interval := r.Interval()
	if interval <= 0 {
		return nil
	}
	if last, err := r.store.LastSpeedtest(ctx); err == nil && r.now().Sub(last.StartedAt) < interval {
		return nil // noch nicht fällig (z. B. nach einem Neustart)
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if !r.begin() {
		return nil // läuft bereits (Button)
	}
	defer r.end()
	events.Publish(r.pub, events.TopicSpeedtest)
	return r.measure(ctx, store.TriggerScheduled)
}

// NextDelay ist die Wartezeit bis zur nächsten fälligen Messung.
func (r *Runner) NextDelay() time.Duration {
	interval := r.Interval()
	if interval <= 0 {
		return idleRecheck
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	last, err := r.store.LastSpeedtest(ctx)
	if err != nil {
		return max(interval, minDelay)
	}
	return max(last.StartedAt.Add(interval).Sub(r.now()), minDelay)
}

func (r *Runner) measure(ctx context.Context, trigger string) error {
	ctx, cancel := context.WithTimeout(ctx, maxRun)
	defer cancel()
	start := r.now()
	res, err := r.client.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil // Herunterfahren – kein Ergebnis speichern
	}
	t := store.Speedtest{StartedAt: start, DurationMS: r.now().Sub(start).Milliseconds(), Trigger: trigger,
		Colo: res.Colo, City: res.City, Country: res.Country, IP: res.IP,
		DownloadBytes: res.DownloadBytes, UploadBytes: res.UploadBytes}
	if err != nil {
		t.ErrorMsg = i18n.FromError(err)
		r.log.Warn("Speedtest fehlgeschlagen", "err", err)
	} else {
		t.DownloadMbps, t.UploadMbps = &res.DownloadMbps, &res.UploadMbps
		t.LatencyMS, t.JitterMS = &res.LatencyMS, &res.JitterMS
		r.log.Info("Speedtest", "download_mbps", round1(res.DownloadMbps), "upload_mbps", round1(res.UploadMbps),
			"latency_ms", round1(res.LatencyMS), "colo", res.Colo, "trigger", trigger)
	}
	// Speichern auch dann, wenn die Messung an der Zeitgrenze abgebrochen wurde.
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelSave()
	if _, err := r.store.InsertSpeedtest(saveCtx, t); err != nil {
		return err
	}
	if _, err := r.store.PruneSpeedtests(saveCtx, r.now().Add(-Retention)); err != nil {
		r.log.Warn("Alte Speedtests löschen fehlgeschlagen", "err", err)
	}
	return nil
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }
