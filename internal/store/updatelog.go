package store

import (
	"context"
	"database/sql"
	"time"
)

// Ergebnisse im Update-Log.
const (
	ResultCreated = "created"
	ResultUpdated = "updated"
	ResultError   = "error"
)

// Auslöser eines Abgleichs.
const (
	TriggerScheduled   = "scheduled"
	TriggerManual      = "manual"
	TriggerRecordSaved = "record_saved"
)

type UpdateLogEntry struct {
	ID         int64     `json:"id"`
	RecordID   *int64    `json:"record_id,omitempty"` // nil, wenn der Record gelöscht wurde
	RecordName string    `json:"record_name"`
	RecordType string    `json:"record_type"`
	Trigger    string    `json:"trigger"`
	Result     string    `json:"result"`
	OldIP      string    `json:"old_ip,omitempty"`
	NewIP      string    `json:"new_ip,omitempty"`
	Message    string    `json:"message,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) InsertUpdateLog(ctx context.Context, e UpdateLogEntry) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO update_log (record_id, record_name, record_type, trigger, result,
			old_ip, new_ip, message, created_at) VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?)`,
		e.RecordID, e.RecordName, e.RecordType, e.Trigger, e.Result, e.OldIP, e.NewIP, e.Message,
		formatTime(e.CreatedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListUpdateLog liefert die jüngsten Einträge; recordID > 0 filtert auf einen Record.
func (s *Store) ListUpdateLog(ctx context.Context, recordID int64, limit int) ([]UpdateLogEntry, error) {
	q := `SELECT id, record_id, record_name, record_type, trigger, result, old_ip, new_ip,
		message, created_at FROM update_log`
	args := []any{}
	if recordID > 0 {
		q += ` WHERE record_id = ?`
		args = append(args, recordID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
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
			created           string
		)
		if err := rows.Scan(&e.ID, &recID, &e.RecordName, &e.RecordType, &e.Trigger, &e.Result,
			&oldIP, &newIP, &msg, &created); err != nil {
			return nil, err
		}
		if recID.Valid {
			e.RecordID = &recID.Int64
		}
		e.OldIP, e.NewIP, e.Message = oldIP.String, newIP.String, msg.String
		if e.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
