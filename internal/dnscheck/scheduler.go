package dnscheck

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

// ErrNothingToCheck: der Record hat noch keine IP (und ist nicht proxied).
var ErrNothingToCheck = errors.New("record has no IP to check")

type recordStore interface {
	GetRecord(ctx context.Context, id int64) (store.Record, error)
	SetRecordPropagation(ctx context.Context, id int64, result []byte) error
}

// Scheduler prüft nach einer Änderung wiederholt, bis alle Server den neuen
// Stand liefern oder die längste sinnvolle Wartezeit (TTL + 2 min) um ist.
type Scheduler struct {
	checker *Checker
	store   recordStore
	log     *slog.Logger
	// Pub wird nach jeder Prüfung informiert (optional).
	Pub events.Publisher

	InitialDelay time.Duration // erste Prüfung nach der Änderung
	Interval     time.Duration // danach
	Grace        time.Duration // zusätzlich zur TTL
	MaxWatch     time.Duration // Obergrenze

	ctx     context.Context
	mu      sync.Mutex
	watches map[int64]*watch
	wg      sync.WaitGroup
}

type watch struct{ cancel context.CancelFunc }

// NewScheduler: ctx beendet alle laufenden Prüfungen (Shutdown).
func NewScheduler(ctx context.Context, c *Checker, st recordStore, log *slog.Logger) *Scheduler {
	return &Scheduler{checker: c, store: st, log: log, ctx: ctx,
		InitialDelay: 10 * time.Second, Interval: 30 * time.Second, Grace: 2 * time.Minute, MaxWatch: time.Hour,
		watches: map[int64]*watch{}}
}

// Watch startet (bzw. startet neu) die wiederholte Prüfung eines Records.
// Kehrt sofort zurück.
func (s *Scheduler) Watch(id int64) {
	ctx, cancel := context.WithCancel(s.ctx)
	w := &watch{cancel: cancel}
	s.mu.Lock()
	if old := s.watches[id]; old != nil {
		old.cancel()
	}
	s.watches[id] = w
	s.mu.Unlock()

	s.wg.Go(func() {
		defer func() {
			cancel()
			s.mu.Lock()
			if s.watches[id] == w {
				delete(s.watches, id)
			}
			s.mu.Unlock()
		}()
		s.run(ctx, id)
	})
}

// Watching meldet, ob für den Record gerade wiederholt geprüft wird.
func (s *Scheduler) Watching(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watches[id] != nil
}

// Wait wartet, bis alle laufenden Prüfungen beendet sind.
func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) run(ctx context.Context, id int64) {
	var deadline time.Time
	delay := s.InitialDelay
	for {
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		rec, err := s.store.GetRecord(ctx, id)
		if err != nil {
			return // gelöscht oder Shutdown
		}
		if deadline.IsZero() {
			deadline = time.Now().Add(s.watchFor(rec))
		}
		res, err := s.check(ctx, rec)
		if err != nil {
			if !errors.Is(err, ErrNothingToCheck) && ctx.Err() == nil {
				s.log.Warn("DNS-Verbreitungsprüfung fehlgeschlagen", "record", rec.Name, "type", rec.Type, "err", err)
			}
			return
		}
		if res.Status == StatusPropagated || !time.Now().Add(s.Interval).Before(deadline) {
			s.log.Debug("DNS-Verbreitung", "record", rec.Name, "type", rec.Type, "status", res.Status,
				"matching", res.Matching, "total", res.Total)
			return
		}
		delay = s.Interval
	}
}

// watchFor: so lange können Resolver den alten Stand noch im Cache haben.
func (s *Scheduler) watchFor(rec store.Record) time.Duration {
	ttl := time.Duration(rec.TTL) * time.Second
	if rec.Proxied || rec.TTL <= 1 {
		ttl = 5 * time.Minute // "Auto" bei Cloudflare
	}
	return min(ttl+s.Grace, s.MaxWatch)
}

// CheckNow prüft sofort einmal und speichert das Ergebnis.
func (s *Scheduler) CheckNow(ctx context.Context, id int64) (Result, error) {
	rec, err := s.store.GetRecord(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return s.check(ctx, rec)
}

func (s *Scheduler) check(ctx context.Context, rec store.Record) (Result, error) {
	if rec.CurrentIP == "" && !rec.Proxied {
		return Result{}, ErrNothingToCheck
	}
	res := s.checker.Check(ctx, Query{Name: rec.Name, Type: rec.Type, Zone: rec.ZoneName,
		Expected: rec.CurrentIP, Proxied: rec.Proxied})
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	b, err := json.Marshal(res)
	if err != nil {
		return Result{}, err
	}
	if err := s.store.SetRecordPropagation(ctx, rec.ID, b); err != nil {
		return Result{}, err
	}
	events.Publish(s.Pub, events.TopicRecords)
	return res, nil
}
