package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Sessions werden nur als Hash des Tokens gespeichert; das Klartext-Token
// existiert ausschließlich im Cookie des Browsers.

func (s *Store) CreateSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, created_at, expires_at) VALUES (?, ?, ?)`,
		tokenHash, formatTime(time.Now()), formatTime(expiresAt))
	return err
}

// SessionValid meldet, ob eine nicht abgelaufene Session zum Hash existiert.
func (s *Store) SessionValid(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var exp string
	err := s.db.QueryRowContext(ctx,
		`SELECT expires_at FROM sessions WHERE token_hash = ?`, tokenHash).Scan(&exp)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	t, err := parseTime(exp)
	if err != nil {
		return false, err
	}
	return now.Before(t), nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// PurgeExpiredSessions entfernt abgelaufene Sessions.
func (s *Store) PurgeExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, formatTime(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
