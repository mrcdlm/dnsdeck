// Package scheduler führt periodische Jobs aus.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	Name string
	// Interval wird vor jedem Warten neu abgefragt, damit geänderte
	// Einstellungen ohne Neustart greifen.
	Interval func() time.Duration
	Run      func(ctx context.Context) error
}

type Scheduler struct {
	log  *slog.Logger
	jobs []Job
	wg   sync.WaitGroup

	mu      sync.Mutex
	resched []chan struct{}
}

func New(log *slog.Logger) *Scheduler { return &Scheduler{log: log} }

func (s *Scheduler) Add(j Job) { s.jobs = append(s.jobs, j) }

// Start startet alle Jobs (jeweils sofort ein erster Lauf) und kehrt zurück.
// Die Jobs enden, wenn ctx abgebrochen wird; Wait wartet darauf.
func (s *Scheduler) Start(ctx context.Context) {
	for _, j := range s.jobs {
		ch := make(chan struct{}, 1)
		s.mu.Lock()
		s.resched = append(s.resched, ch)
		s.mu.Unlock()
		s.wg.Go(func() { s.loop(ctx, j, ch) })
	}
}

// Reschedule lässt alle Jobs ihr Intervall neu berechnen (z. B. nach einer
// Änderung der Einstellungen). Ist das neue Intervall seit dem letzten Lauf
// bereits verstrichen, läuft der Job sofort.
func (s *Scheduler) Reschedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.resched {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) loop(ctx context.Context, j Job, resched <-chan struct{}) {
	for {
		start := time.Now()
		if err := j.Run(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("Job fehlgeschlagen", "job", j.Name, "err", err)
		} else {
			s.log.Debug("Job ausgeführt", "job", j.Name, "duration", time.Since(start))
		}

		t := time.NewTimer(j.Interval())
	wait:
		for {
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
				break wait
			case <-resched:
				t.Stop()
				t = time.NewTimer(max(0, j.Interval()-time.Since(start)))
			}
		}
	}
}
