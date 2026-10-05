package store

import (
	"context"
	"database/sql"
	"encoding/json"
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

// TunnelSegment ist ein Zeitabschnitt mit gleichem Status (siehe Segment).
type TunnelSegment = Segment

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

// RecordTunnelStatus hält einen beobachteten Tunnel-Status fest (siehe
// recordSegment). prev ist der Status der vorherigen Beobachtung ("" beim
// ersten Mal).
func (s *Store) RecordTunnelStatus(ctx context.Context, tunnelID, status string, now time.Time, maxGap time.Duration) (prev string, err error) {
	return s.recordSegment(ctx, tunnelSegments, tunnelID, status, now, maxGap)
}

// TunnelSegments liefert die Abschnitte eines Tunnels, die nach since enden,
// aufsteigend sortiert.
func (s *Store) TunnelSegments(ctx context.Context, tunnelID string, since time.Time) ([]TunnelSegment, error) {
	return s.segments(ctx, tunnelSegments, tunnelID, since)
}

// PurgeTunnelSegments löscht Abschnitte, die vor before endeten.
func (s *Store) PurgeTunnelSegments(ctx context.Context, before time.Time) (int64, error) {
	return s.purgeSegments(ctx, tunnelSegments, before)
}

// TunnelChange ist ein Statuswechsel, abgeleitet aus den Abschnitten.
type TunnelChange struct {
	TunnelID   string    `json:"tunnel_id"`
	TunnelName string    `json:"tunnel_name"`
	From       string    `json:"from,omitempty"` // leer = erstmals beobachtet
	To         string    `json:"to"`
	At         time.Time `json:"at"`
}

// TunnelChangeFilter schränkt TunnelStatusChanges ein; Nullwerte = kein Filter.
type TunnelChangeFilter struct {
	TunnelID string
	Before   time.Time // nur Wechsel vor diesem Zeitpunkt (Blättern)
}

// TunnelStatusChanges liefert Statuswechsel (neueste zuerst). Aufeinander
// folgende Abschnitte gleichen Status (z. B. nach einer Lücke) zählen nicht
// als Wechsel.
func (s *Store) TunnelStatusChanges(ctx context.Context, f TunnelChangeFilter, limit int) ([]TunnelChange, error) {
	before := ""
	if !f.Before.IsZero() {
		before = formatTime(f.Before)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.tunnel_id, COALESCE(t.name, c.tunnel_id), COALESCE(c.prev, ''), c.status, c.started_at
		 FROM (
			SELECT tunnel_id, status, started_at,
				LAG(status) OVER (PARTITION BY tunnel_id ORDER BY started_at, id) AS prev
			FROM tunnel_status_segments
		 ) c
		 LEFT JOIN tunnels t ON t.id = c.tunnel_id
		 WHERE (c.prev IS NULL OR c.prev <> c.status)
			AND (?1 = '' OR c.tunnel_id = ?1) AND (?2 = '' OR c.started_at < ?2)
		 ORDER BY c.started_at DESC LIMIT ?3`, f.TunnelID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TunnelChange{}
	for rows.Next() {
		var (
			c  TunnelChange
			at string
		)
		if err := rows.Scan(&c.TunnelID, &c.TunnelName, &c.From, &c.To, &at); err != nil {
			return nil, err
		}
		if c.At, err = parseTime(at); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
