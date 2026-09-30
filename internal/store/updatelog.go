package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

// Ergebnisse im Update-Log.
const (
	ResultCreated   = "created"
	ResultAdopted   = "adopted" // bestehenden Eintrag unverändert übernommen
	ResultUpdated   = "updated"
	ResultRecovered = "recovered" // nach Fehler wieder erfolgreich
	ResultError     = "error"
)

// Auslöser eines Abgleichs.
const (
	TriggerScheduled   = "scheduled"
	TriggerManual      = "manual"
	TriggerRecordSaved = "record_saved"
)

type UpdateLogEntry struct {
	ID         int64  `json:"id"`
	RecordID   *int64 `json:"record_id,omitempty"` // nil, wenn der Record gelöscht wurde
	RecordName string `json:"record_name"`
	RecordType string `json:"record_type"`
	Trigger    string `json:"trigger"`
	Result     string `json:"result"`
	OldIP      string `json:"old_ip,omitempty"`
	NewIP      string `json:"new_ip,omitempty"`
	// Message: gerenderter Text (von der API gesetzt bzw. Alttext);
	// MessageMsg: übersetzbare Meldung.
	Message    string    `json:"message,omitempty"`
	MessageMsg i18n.Msg  `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) InsertUpdateLog(ctx context.Context, e UpdateLogEntry) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO update_log (record_id, record_name, record_type, trigger, result,
			old_ip, new_ip, message, message_i18n, created_at)
		 VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?)`,
		e.RecordID, e.RecordName, e.RecordType, e.Trigger, e.Result, e.OldIP, e.NewIP, e.Message,
		i18n.Encode(e.MessageMsg), formatTime(e.CreatedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateLogFilter schränkt ListUpdateLog ein; Nullwerte = kein Filter.
type UpdateLogFilter struct {
	RecordID int64
	Result   string // created | adopted | updated | recovered | error
	BeforeID int64  // nur Einträge mit kleinerer ID (Blättern)
}

// ListUpdateLog liefert die jüngsten Einträge (neueste zuerst).
func (s *Store) ListUpdateLog(ctx context.Context, f UpdateLogFilter, limit int) ([]UpdateLogEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, record_id, record_name, record_type, trigger, result, old_ip, new_ip,
			message, message_i18n, created_at FROM update_log
		 WHERE (?1 = 0 OR record_id = ?1) AND (?2 = '' OR result = ?2) AND (?3 = 0 OR id < ?3)
		 ORDER BY id DESC LIMIT ?4`, f.RecordID, f.Result, f.BeforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UpdateLogEntry{}
	for rows.Next() {
		var (
			e                 UpdateLogEntry
			recID             sql.NullInt64
			oldIP, newIP, msg sql.NullString
			msgI18n           sql.NullString
			created           string
		)
		if err := rows.Scan(&e.ID, &recID, &e.RecordName, &e.RecordType, &e.Trigger, &e.Result,
			&oldIP, &newIP, &msg, &msgI18n, &created); err != nil {
			return nil, err
		}
		if recID.Valid {
			e.RecordID = &recID.Int64
		}
		e.OldIP, e.NewIP, e.Message = oldIP.String, newIP.String, msg.String
		e.MessageMsg = i18n.Decode(msgI18n.String)
		if e.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
