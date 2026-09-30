package scheduler

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerRunsAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var runs atomic.Int32
	s := New(slog.New(slog.DiscardHandler))
	s.Add(Job{
		Name:     "test",
		Interval: func() time.Duration { return 5 * time.Millisecond },
		Run: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	})
	s.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for runs.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	s.Wait()
	if runs.Load() < 3 {
		t.Fatalf("nur %d Läufe", runs.Load())
	}
}

func TestSchedulerFirstRunImmediate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, 1)
	s := New(slog.New(slog.DiscardHandler))
	s.Add(Job{
		Name:     "sofort",
		Interval: func() time.Duration { return time.Hour },
		Run:      func(context.Context) error { done <- struct{}{}; return nil },
	})
	s.Start(ctx)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("erster Lauf nicht sofort")
	}
	cancel()
	s.Wait()
}

func TestReschedule(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var interval atomic.Int64
	interval.Store(int64(time.Hour))
	var runs atomic.Int32
	s := New(slog.New(slog.DiscardHandler))
	s.Add(Job{
		Name:     "umplanen",
		Interval: func() time.Duration { return time.Duration(interval.Load()) },
		Run:      func(context.Context) error { runs.Add(1); return nil },
	})
	s.Start(ctx)
	for runs.Load() < 1 {
		time.Sleep(time.Millisecond)
	}
	// Intervall verkürzen → ohne Reschedule würde erst nach 1 h wieder laufen
	interval.Store(int64(10 * time.Millisecond))
	s.Reschedule()
	deadline := time.Now().Add(2 * time.Second)
	for runs.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runs.Load() < 3 {
		t.Fatalf("nach Reschedule nur %d Läufe", runs.Load())
	}
	cancel()
	s.Wait()
}
