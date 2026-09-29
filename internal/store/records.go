package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrConflict wird bei Verletzung einer Eindeutigkeitsbedingung zurückgegeben.
var ErrConflict = errors.New("existiert bereits")

// Record-Status nach dem letzten Abgleich.
const (
	RecordPending = "pending" // noch nicht abgeglichen
	RecordOK      = "ok"      // stimmt mit aktueller IP überein
	RecordError   = "error"   // letzter Abgleich fehlgeschlagen
	RecordSkipped = "skipped" // keine IP der passenden Familie bekannt
	RecordPaused  = "paused"  // deaktiviert
)

type Record struct {
	ID               int64      `json:"id"`
	Provider         string     `json:"provider"`
	ZoneID           string     `json:"zone_id"`
	ZoneName         string     `json:"zone_name"`
	Name             string     `json:"name"`
	Type             string     `json:"type"`
	Proxied          bool       `json:"proxied"`
	TTL              int        `json:"ttl"`
	Enabled          bool       `json:"enabled"`
	ProviderRecordID string     `json:"-"`
	CurrentIP        string     `json:"current_ip,omitempty"`
	Status           string     `json:"status"`
	Message          string     `json:"message,omitempty"`
	LastCheckedAt    *time.Time `json:"last_checked_at,omitempty"`
	LastChangedAt    *time.Time `json:"last_changed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// RecordSyncState ist das Ergebnis eines Abgleichs.
type RecordSyncState struct {
	ProviderRecordID string
	CurrentIP        string
	Status           string
	Message          string
	CheckedAt        time.Time
	Changed          bool // IP/Einstellungen wurden beim Provider geändert
}

const recordCols = `id, provider, zone_id, zone_name, name, type, proxied, ttl, enabled,
	provider_record_id, current_ip, status, message, last_checked_at, last_changed_at,
	created_at, updated_at`

func (s *Store) CreateRecord(ctx context.Context, r Record) (Record, error) {
	now := formatTime(time.Now())
	if r.Provider == "" {
		r.Provider = "cloudflare"
	}
	status := RecordPending
	if !r.Enabled {
		status = RecordPaused
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO records (provider, zone_id, zone_name, name, type, proxied, ttl, enabled,
			status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Provider, r.ZoneID, r.ZoneName, r.Name, r.Type, r.Proxied, r.TTL, r.Enabled, status, now, now)
	if err != nil {
		return Record{}, mapConstraint(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Record{}, err
	}
	return s.GetRecord(ctx, id)
}

// UpdateRecordSettings ändert die vom Benutzer pflegbaren Felder. Ändern sich
// Zone, Name oder Typ, wird die gemerkte Provider-ID verworfen.
func (s *Store) UpdateRecordSettings(ctx context.Context, r Record) (Record, error) {
	old, err := s.GetRecord(ctx, r.ID)
	if err != nil {
		return Record{}, err
	}
	providerID := sql.NullString{String: old.ProviderRecordID, Valid: old.ProviderRecordID != ""}
	if old.ZoneID != r.ZoneID || old.Name != r.Name || old.Type != r.Type {
		providerID = sql.NullString{}
	}
	status, message := RecordPending, sql.NullString{}
	if !r.Enabled {
		status = RecordPaused
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE records SET zone_id = ?, zone_name = ?, name = ?, type = ?, proxied = ?, ttl = ?,
			enabled = ?, provider_record_id = ?, status = ?, message = ?, updated_at = ?
		 WHERE id = ?`,
		r.ZoneID, r.ZoneName, r.Name, r.Type, r.Proxied, r.TTL, r.Enabled, providerID,
		status, message, formatTime(time.Now()), r.ID)
	if err != nil {
		return Record{}, mapConstraint(err)
	}
	return s.GetRecord(ctx, r.ID)
}

func (s *Store) SetRecordSyncState(ctx context.Context, id int64, st RecordSyncState) error {
	var changedAt any
	if st.Changed {
		changedAt = formatTime(st.CheckedAt)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE records SET
			provider_record_id = COALESCE(NULLIF(?, ''), provider_record_id),
			current_ip = COALESCE(NULLIF(?, ''), current_ip),
			status = ?, message = NULLIF(?, ''), last_checked_at = ?,
			last_changed_at = COALESCE(?, last_changed_at)
		 WHERE id = ?`,
		st.ProviderRecordID, st.CurrentIP, st.Status, st.Message,
		formatTime(st.CheckedAt), changedAt, id)
	return err
}

func (s *Store) DeleteRecord(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM records WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetRecord(ctx context.Context, id int64) (Record, error) {
	r, err := scanRecord(s.db.QueryRowContext(ctx, `SELECT `+recordCols+` FROM records WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return r, err
}

func (s *Store) ListRecords(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordCols+` FROM records ORDER BY name, type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanRecord(sc scanner) (Record, error) {
	var (
		r                          Record
		providerID, currentIP, msg sql.NullString
		checked, changed           sql.NullString
		created, updated           string
	)
	err := sc.Scan(&r.ID, &r.Provider, &r.ZoneID, &r.ZoneName, &r.Name, &r.Type, &r.Proxied,
		&r.TTL, &r.Enabled, &providerID, &currentIP, &r.Status, &msg, &checked, &changed,
		&created, &updated)
	if err != nil {
		return Record{}, err
	}
	r.ProviderRecordID, r.CurrentIP, r.Message = providerID.String, currentIP.String, msg.String
	if r.LastCheckedAt, err = parseNullTime(checked); err != nil {
		return Record{}, err
	}
	if r.LastChangedAt, err = parseNullTime(changed); err != nil {
		return Record{}, err
	}
	if r.CreatedAt, err = parseTime(created); err != nil {
		return Record{}, err
	}
	if r.UpdatedAt, err = parseTime(updated); err != nil {
		return Record{}, err
	}
	return r, nil
}

func parseNullTime(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func mapConstraint(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrConflict
	}
	return err
}
