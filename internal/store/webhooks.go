package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
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
	// LastError: gerenderter Text (von der API gesetzt bzw. Alttext);
	// LastErrorMsg: übersetzbare Meldung.
	LastError    string    `json:"last_error,omitempty"`
	LastErrorMsg i18n.Msg  `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

const webhookCols = `id, name, enabled, method, url, headers, content_type, body_template, events,
	last_sent_at, last_error, last_error_i18n, created_at, updated_at`

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

// UpdateWebhook ändert die Konfiguration. Ändert sich, was gesendet wird
// (Methode, URL, Header, Body), wird der letzte Zustellstatus zurückgesetzt –
// er bezog sich auf die alte Konfiguration. Name, Aktiv-Schalter und
// Ereignisauswahl lassen ihn unberührt.
func (s *Store) UpdateWebhook(ctx context.Context, w Webhook) (Webhook, error) {
	headers, events := marshalWebhookLists(w)
	// SET-Ausdrücke sehen die alten Spaltenwerte.
	res, err := s.db.ExecContext(ctx,
		`UPDATE webhooks SET
			last_error   = CASE WHEN method <> ?3 OR url <> ?4 OR headers <> ?5 OR content_type <> ?6 OR body_template <> ?7
			               THEN NULL ELSE last_error END,
			last_error_i18n = CASE WHEN method <> ?3 OR url <> ?4 OR headers <> ?5 OR content_type <> ?6 OR body_template <> ?7
			               THEN NULL ELSE last_error_i18n END,
			last_sent_at = CASE WHEN method <> ?3 OR url <> ?4 OR headers <> ?5 OR content_type <> ?6 OR body_template <> ?7
			               THEN NULL ELSE last_sent_at END,
			name = ?1, enabled = ?2, method = ?3, url = ?4, headers = ?5, content_type = ?6,
			body_template = ?7, events = ?8, updated_at = ?9
		 WHERE id = ?10`,
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
		_, err = s.db.ExecContext(ctx, `UPDATE webhooks SET last_error = NULL, last_error_i18n = ? WHERE id = ?`,
			i18n.Encode(i18n.FromError(deliveryErr)), id)
	} else {
		_, err = s.db.ExecContext(ctx, `UPDATE webhooks SET last_sent_at = ?, last_error = NULL, last_error_i18n = NULL WHERE id = ?`,
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
		lastErI18n       sql.NullString
		created, updated string
	)
	if err := sc.Scan(&w.ID, &w.Name, &w.Enabled, &w.Method, &w.URL, &headers, &w.ContentType,
		&w.BodyTemplate, &events, &lastSent, &lastEr, &lastErI18n, &created, &updated); err != nil {
		return Webhook{}, err
	}
	if err := json.Unmarshal([]byte(headers), &w.Headers); err != nil {
		return Webhook{}, err
	}
	if err := json.Unmarshal([]byte(events), &w.Events); err != nil {
		return Webhook{}, err
	}
	w.LastError, w.LastErrorMsg = lastEr.String, i18n.Decode(lastErI18n.String)
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
