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
}

func New(log *slog.Logger) *Scheduler { return &Scheduler{log: log} }

func (s *Scheduler) Add(j Job) { s.jobs = append(s.jobs, j) }

// Start startet alle Jobs (jeweils sofort ein erster Lauf) und kehrt zurück.
// Die Jobs enden, wenn ctx abgebrochen wird; Wait wartet darauf.
func (s *Scheduler) Start(ctx context.Context) {
	for _, j := range s.jobs {
		s.wg.Go(func() { s.loop(ctx, j) })
	}
}

func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) loop(ctx context.Context, j Job) {
	for {
		start := time.Now()
		if err := j.Run(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("Job fehlgeschlagen", "job", j.Name, "err", err)
		} else {
			s.log.Debug("Job ausgeführt", "job", j.Name, "duration", time.Since(start))
		}

		t := time.NewTimer(j.Interval())
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}
