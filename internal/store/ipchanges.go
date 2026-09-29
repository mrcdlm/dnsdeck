package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type IPChange struct {
	ID         int64     `json:"id"`
	Family     string    `json:"family"` // "ipv4" oder "ipv6"
	IP         string    `json:"ip"`
	PreviousIP string    `json:"previous_ip,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
}

func (s *Store) InsertIPChange(ctx context.Context, c IPChange) (int64, error) {
	var prev sql.NullString
	if c.PreviousIP != "" {
		prev = sql.NullString{String: c.PreviousIP, Valid: true}
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO ip_changes (family, ip, previous_ip, detected_at) VALUES (?, ?, ?, ?)`,
		c.Family, c.IP, prev, formatTime(c.DetectedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LatestIPChange liefert den jüngsten Eintrag einer Familie oder ErrNotFound.
func (s *Store) LatestIPChange(ctx context.Context, family string) (IPChange, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, family, ip, previous_ip, detected_at FROM ip_changes
		 WHERE family = ? ORDER BY id DESC LIMIT 1`, family)
	c, err := scanIPChange(row)
	if errors.Is(err, sql.ErrNoRows) {
		return IPChange{}, ErrNotFound
	}
	return c, err
}

// ListIPChanges liefert die jüngsten Einträge (neueste zuerst).
func (s *Store) ListIPChanges(ctx context.Context, limit int) ([]IPChange, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, family, ip, previous_ip, detected_at FROM ip_changes
		 ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []IPChange{}
	for rows.Next() {
		c, err := scanIPChange(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanIPChange(r scanner) (IPChange, error) {
	var (
		c    IPChange
		prev sql.NullString
		at   string
	)
	if err := r.Scan(&c.ID, &c.Family, &c.IP, &prev, &at); err != nil {
		return IPChange{}, err
	}
	c.PreviousIP = prev.String
	t, err := parseTime(at)
	if err != nil {
		return IPChange{}, err
	}
	c.DetectedAt = t
	return c, nil
}
