package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Webhook struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Enabled      bool       `json:"enabled"`
	Method       string     `json:"method"`
	URL          string     `json:"url"`
	Headers      []Header   `json:"headers"`
	ContentType  string     `json:"content_type"`
	BodyTemplate string     `json:"body_template"`
	Events       []string   `json:"events"`
	LastSentAt   *time.Time `json:"last_sent_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

const webhookCols = `id, name, enabled, method, url, headers, content_type, body_template, events,
	last_sent_at, last_error, created_at, updated_at`

func (s *Store) CreateWebhook(ctx context.Context, w Webhook) (Webhook, error) {
	headers, events := marshalWebhookLists(w)
	now := formatTime(time.Now())
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO webhooks (name, enabled, method, url, headers, content_type, body_template, events,
			created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.Name, w.Enabled, w.Method, w.URL, headers, w.ContentType, w.BodyTemplate, events, now, now)
	if err != nil {
		return Webhook{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Webhook{}, err
	}
	return s.GetWebhook(ctx, id)
}

// UpdateWebhook ändert die Konfiguration; der letzte Zustellstatus wird
// zurückgesetzt, weil er sich auf die alte Konfiguration bezog.
func (s *Store) UpdateWebhook(ctx context.Context, w Webhook) (Webhook, error) {
	headers, events := marshalWebhookLists(w)
	res, err := s.db.ExecContext(ctx,
		`UPDATE webhooks SET name = ?, enabled = ?, method = ?, url = ?, headers = ?, content_type = ?,
			body_template = ?, events = ?, last_error = NULL, updated_at = ? WHERE id = ?`,
		w.Name, w.Enabled, w.Method, w.URL, headers, w.ContentType, w.BodyTemplate, events,
		formatTime(time.Now()), w.ID)
	if err != nil {
		return Webhook{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Webhook{}, ErrNotFound
	}
	return s.GetWebhook(ctx, w.ID)
}

func (s *Store) DeleteWebhook(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetWebhookResult hält das Ergebnis der letzten Zustellung fest.
func (s *Store) SetWebhookResult(ctx context.Context, id int64, at time.Time, deliveryErr error) error {
	var err error
	if deliveryErr != nil {
		_, err = s.db.ExecContext(ctx, `UPDATE webhooks SET last_error = ? WHERE id = ?`, deliveryErr.Error(), id)
	} else {
		_, err = s.db.ExecContext(ctx, `UPDATE webhooks SET last_sent_at = ?, last_error = NULL WHERE id = ?`,
			formatTime(at), id)
	}
	return err
}

func (s *Store) GetWebhook(ctx context.Context, id int64) (Webhook, error) {
	w, err := scanWebhook(s.db.QueryRowContext(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Webhook{}, ErrNotFound
	}
	return w, err
}

func (s *Store) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+webhookCols+` FROM webhooks ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Webhook{}
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func marshalWebhookLists(w Webhook) (headers, events string) {
	if w.Headers == nil {
		w.Headers = []Header{}
	}
	if w.Events == nil {
		w.Events = []string{}
	}
	h, _ := json.Marshal(w.Headers)
	e, _ := json.Marshal(w.Events)
	return string(h), string(e)
}

func scanWebhook(sc scanner) (Webhook, error) {
	var (
		w                Webhook
		headers, events  string
		lastSent, lastEr sql.NullString
		created, updated string
	)
	if err := sc.Scan(&w.ID, &w.Name, &w.Enabled, &w.Method, &w.URL, &headers, &w.ContentType,
		&w.BodyTemplate, &events, &lastSent, &lastEr, &created, &updated); err != nil {
		return Webhook{}, err
	}
	if err := json.Unmarshal([]byte(headers), &w.Headers); err != nil {
		return Webhook{}, err
	}
	if err := json.Unmarshal([]byte(events), &w.Events); err != nil {
		return Webhook{}, err
	}
	w.LastError = lastEr.String
	var err error
	if w.LastSentAt, err = parseNullTime(lastSent); err != nil {
		return Webhook{}, err
	}
	if w.CreatedAt, err = parseTime(created); err != nil {
		return Webhook{}, err
	}
	if w.UpdatedAt, err = parseTime(updated); err != nil {
		return Webhook{}, err
	}
	return w, nil
}
