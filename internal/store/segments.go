package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Segment ist ein Zeitabschnitt mit gleichem Status (Tunnel oder Prüfung).
type Segment struct {
	Status     string
	StartedAt  time.Time
	LastSeenAt time.Time
}

// segmentTable beschreibt eine Tabelle mit Statusabschnitten. Tabellen- und
// Spaltennamen sind feste Konstanten, nie Eingaben.
type segmentTable struct{ table, key string }

var (
	tunnelSegments = segmentTable{"tunnel_status_segments", "tunnel_id"}
	probeSegments  = segmentTable{"probe_status_segments", "probe_id"}
)

// recordSegment hält einen beobachteten Status fest. Bleibt der Status gleich
// und liegt die letzte Beobachtung höchstens maxGap zurück, wird der laufende
// Abschnitt verlängert, sonst ein neuer begonnen. prev ist der Status der
// vorherigen Beobachtung ("" beim ersten Mal).
func (s *Store) recordSegment(ctx context.Context, t segmentTable, key any, status string, now time.Time, maxGap time.Duration) (prev string, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var (
		id       int64
		lastSeen string
	)
	err = tx.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT id, status, last_seen_at FROM %s WHERE %s = ? ORDER BY started_at DESC, id DESC LIMIT 1`,
		t.table, t.key), key).Scan(&id, &prev, &lastSeen)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		prev = ""
	case err != nil:
		return "", err
	}

	extend := false
	if prev == status {
		last, err := parseTime(lastSeen)
		if err != nil {
			return "", err
		}
		extend = now.Sub(last) <= maxGap
	}
	if extend {
		_, err = tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET last_seen_at = ? WHERE id = ?`, t.table),
			formatTime(now), id)
	} else {
		_, err = tx.ExecContext(ctx, fmt.Sprintf(
			`INSERT INTO %s (%s, status, started_at, last_seen_at) VALUES (?, ?, ?, ?)`, t.table, t.key),
			key, status, formatTime(now), formatTime(now))
	}
	if err != nil {
		return "", err
	}
	return prev, tx.Commit()
}

// segments liefert die Abschnitte zu key, die nach since enden, aufsteigend sortiert.
func (s *Store) segments(ctx context.Context, t segmentTable, key any, since time.Time) ([]Segment, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT status, started_at, last_seen_at FROM %s WHERE %s = ? AND last_seen_at >= ? ORDER BY started_at, id`,
		t.table, t.key), key, formatTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Segment{}
	for rows.Next() {
		var (
			seg         Segment
			start, last string
		)
		if err := rows.Scan(&seg.Status, &start, &last); err != nil {
			return nil, err
		}
		if seg.StartedAt, err = parseTime(start); err != nil {
			return nil, err
		}
		if seg.LastSeenAt, err = parseTime(last); err != nil {
			return nil, err
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}

// purgeSegments löscht Abschnitte, die vor before endeten.
func (s *Store) purgeSegments(ctx context.Context, t segmentTable, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE last_seen_at < ?`, t.table), formatTime(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecordProbeStatus hält einen beobachteten Prüfungsstatus fest (siehe recordSegment).
func (s *Store) RecordProbeStatus(ctx context.Context, probeID int64, status string, now time.Time, maxGap time.Duration) error {
	_, err := s.recordSegment(ctx, probeSegments, probeID, status, now, maxGap)
	return err
}

// ProbeSegments liefert die Abschnitte einer Prüfung, die nach since enden.
func (s *Store) ProbeSegments(ctx context.Context, probeID int64, since time.Time) ([]Segment, error) {
	return s.segments(ctx, probeSegments, probeID, since)
}

// PurgeProbeSegments löscht Abschnitte, die vor before endeten.
func (s *Store) PurgeProbeSegments(ctx context.Context, before time.Time) (int64, error) {
	return s.purgeSegments(ctx, probeSegments, before)
}
