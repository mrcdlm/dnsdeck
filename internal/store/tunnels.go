package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Tunnel ist der zuletzt bekannte Stand eines Cloudflare-Tunnels.
type Tunnel struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Status          string          `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
	ConnsActiveAt   *time.Time      `json:"conns_active_at,omitempty"`
	ConnsInactiveAt *time.Time      `json:"conns_inactive_at,omitempty"`
	Connections     json.RawMessage `json:"connections"`
	RemoteConfig    bool            `json:"remote_config"`
	FirstSeenAt     time.Time       `json:"first_seen_at"`
	LastSeenAt      time.Time       `json:"last_seen_at"`
	RemovedAt       *time.Time      `json:"removed_at,omitempty"`
}

// TunnelSegment ist ein Zeitabschnitt mit gleichem Status.
type TunnelSegment struct {
	Status     string
	StartedAt  time.Time
	LastSeenAt time.Time
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

// UpsertTunnel speichert den aktuellen Stand; ein zuvor entfernter Tunnel
// gilt wieder als vorhanden.
func (s *Store) UpsertTunnel(ctx context.Context, t Tunnel) error {
	conns := t.Connections
	if len(conns) == 0 {
		conns = json.RawMessage("[]")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tunnels (id, name, status, created_at, conns_active_at, conns_inactive_at,
			connections, remote_config, first_seen_at, last_seen_at)
		 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?9)
		 ON CONFLICT(id) DO UPDATE SET
			name = excluded.name, status = excluded.status, created_at = excluded.created_at,
			conns_active_at = excluded.conns_active_at, conns_inactive_at = excluded.conns_inactive_at,
			connections = excluded.connections, remote_config = excluded.remote_config,
			last_seen_at = excluded.last_seen_at, removed_at = NULL`,
		t.ID, t.Name, t.Status, formatTime(t.CreatedAt), nullTime(t.ConnsActiveAt),
		nullTime(t.ConnsInactiveAt), string(conns), t.RemoteConfig, formatTime(t.LastSeenAt))
	return err
}

// MarkTunnelsRemoved markiert alle Tunnels außer present als entfernt.
func (s *Store) MarkTunnelsRemoved(ctx context.Context, present []string, now time.Time) (int64, error) {
	if present == nil {
		present = []string{} // nil würde zu JSON null → NOT IN (NULL) trifft nie zu
	}
	ids, _ := json.Marshal(present)
	res, err := s.db.ExecContext(ctx,
		`UPDATE tunnels SET removed_at = ? WHERE removed_at IS NULL
		 AND id NOT IN (SELECT value FROM json_each(?))`, formatTime(now), string(ids))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListTunnels liefert die Tunnels sortiert nach Name; entfernte nur auf Wunsch.
func (s *Store) ListTunnels(ctx context.Context, includeRemoved bool) ([]Tunnel, error) {
	q := `SELECT id, name, status, created_at, conns_active_at, conns_inactive_at, connections,
		remote_config, first_seen_at, last_seen_at, removed_at FROM tunnels`
	if !includeRemoved {
		q += ` WHERE removed_at IS NULL`
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Tunnel{}
	for rows.Next() {
		var (
			t                               Tunnel
			created, first, last, conns     string
			activeAt, inactiveAt, removedAt sql.NullString
		)
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &created, &activeAt, &inactiveAt, &conns,
			&t.RemoteConfig, &first, &last, &removedAt); err != nil {
			return nil, err
		}
		t.Connections = json.RawMessage(conns)
		for _, p := range []struct {
			dst *time.Time
			src string
		}{{&t.CreatedAt, created}, {&t.FirstSeenAt, first}, {&t.LastSeenAt, last}} {
			if *p.dst, err = parseTime(p.src); err != nil {
				return nil, err
			}
		}
		for _, p := range []struct {
			dst **time.Time
			src sql.NullString
		}{{&t.ConnsActiveAt, activeAt}, {&t.ConnsInactiveAt, inactiveAt}, {&t.RemovedAt, removedAt}} {
			if *p.dst, err = parseNullTime(p.src); err != nil {
				return nil, err
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RecordTunnelStatus hält einen beobachteten Status fest. Bleibt der Status
// gleich und liegt die letzte Beobachtung höchstens maxGap zurück, wird der
// laufende Abschnitt verlängert, sonst ein neuer begonnen. prev ist der Status
// der vorherigen Beobachtung ("" beim ersten Mal).
func (s *Store) RecordTunnelStatus(ctx context.Context, tunnelID, status string, now time.Time, maxGap time.Duration) (prev string, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var (
		id       int64
		lastSeen string
	)
	err = tx.QueryRowContext(ctx,
		`SELECT id, status, last_seen_at FROM tunnel_status_segments
		 WHERE tunnel_id = ? ORDER BY started_at DESC, id DESC LIMIT 1`, tunnelID).Scan(&id, &prev, &lastSeen)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		prev = ""
	case err != nil:
		return "", err
	}

	extend := false
	if prev == status {
		t, err := parseTime(lastSeen)
		if err != nil {
			return "", err
		}
		extend = now.Sub(t) <= maxGap
	}
	if extend {
		_, err = tx.ExecContext(ctx, `UPDATE tunnel_status_segments SET last_seen_at = ? WHERE id = ?`,
			formatTime(now), id)
	} else {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO tunnel_status_segments (tunnel_id, status, started_at, last_seen_at) VALUES (?, ?, ?, ?)`,
			tunnelID, status, formatTime(now), formatTime(now))
	}
	if err != nil {
		return "", err
	}
	return prev, tx.Commit()
}

// TunnelSegments liefert die Abschnitte eines Tunnels, die nach since enden,
// aufsteigend sortiert.
func (s *Store) TunnelSegments(ctx context.Context, tunnelID string, since time.Time) ([]TunnelSegment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT status, started_at, last_seen_at FROM tunnel_status_segments
		 WHERE tunnel_id = ? AND last_seen_at >= ? ORDER BY started_at, id`, tunnelID, formatTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TunnelSegment{}
	for rows.Next() {
		var (
			seg         TunnelSegment
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

// PurgeTunnelSegments löscht Abschnitte, die vor before endeten.
func (s *Store) PurgeTunnelSegments(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tunnel_status_segments WHERE last_seen_at < ?`, formatTime(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
