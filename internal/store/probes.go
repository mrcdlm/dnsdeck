package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

// Status einer Erreichbarkeitsprüfung.
const (
	ProbePending  = "pending"   // noch nicht geprüft
	ProbeUp       = "up"        // erreichbar, Zertifikat in Ordnung
	ProbeExpiring = "expiring"  // erreichbar, Zertifikat läuft bald ab
	ProbeTLSError = "tls_error" // erreichbar, Zertifikat ungültig
	ProbeDown     = "down"      // nicht erreichbar oder HTTP 5xx
	ProbePaused   = "paused"    // deaktiviert
)

// Probe ist eine Erreichbarkeitsprüfung.
type Probe struct {
	ID       int64  `json:"id"`
	RecordID *int64 `json:"record_id,omitempty"`
	// RecordName: Name des zugehörigen Records (nur zur Anzeige).
	RecordName string `json:"record_name,omitempty"`
	URL        string `json:"url"`
	// ExpectedStatus: erwartete HTTP-Statuscodes, z. B. "200" oder "2xx"
	// (leer = jede Antwort unter 500 gilt als erreichbar).
	ExpectedStatus string `json:"expected_status,omitempty"`
	Enabled        bool   `json:"enabled"`
	Status         string `json:"status"`
	HTTPStatus     *int   `json:"http_status,omitempty"`
	LatencyMS      *int   `json:"latency_ms,omitempty"`
	FailCount      int    `json:"fail_count"`
	// Message: von der API gerenderter Text; MessageMsg: übersetzbare Meldung.
	Message       string     `json:"message,omitempty"`
	MessageMsg    i18n.Msg   `json:"-"`
	TLSNotAfter   *time.Time `json:"tls_not_after,omitempty"`
	TLSIssuer     string     `json:"tls_issuer,omitempty"`
	TLSValid      *bool      `json:"tls_valid,omitempty"`
	CertWarnedFor *time.Time `json:"-"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastChangedAt *time.Time `json:"last_changed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ProbeResult ist der neue Stand nach einer Prüfung.
type ProbeResult struct {
	Status        string
	HTTPStatus    *int
	LatencyMS     *int
	FailCount     int
	Message       i18n.Msg
	TLSNotAfter   *time.Time
	TLSIssuer     string
	TLSValid      *bool
	CertWarnedFor *time.Time
	CheckedAt     time.Time
	Changed       bool // Status hat gewechselt
}

const probeCols = `p.id, p.record_id, r.name, p.url, p.enabled, p.status, p.http_status, p.latency_ms,
	p.fail_count, p.message_i18n, p.tls_not_after, p.tls_issuer, p.tls_valid, p.cert_warned_for,
	p.last_checked_at, p.last_changed_at, p.created_at, p.updated_at, p.expected_status`

const probeFrom = ` FROM probes p LEFT JOIN records r ON r.id = p.record_id`

// CreateProbe legt eine eigene Prüfung an (ohne Record).
func (s *Store) CreateProbe(ctx context.Context, url string, enabled bool) (Probe, error) {
	now := formatTime(time.Now())
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO probes (url, enabled, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		url, enabled, initialProbeStatus(enabled), now, now)
	if err != nil {
		return Probe{}, mapConstraint(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Probe{}, err
	}
	return s.GetProbe(ctx, id)
}

// UpdateProbe ändert Adresse und Aktivierung. Ändert sich die Adresse, beginnt
// die Prüfung von vorn.
func (s *Store) UpdateProbe(ctx context.Context, id int64, url string, enabled bool) (Probe, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE probes SET
			status = CASE WHEN url <> ?1 OR enabled <> ?2 THEN ?3 ELSE status END,
			fail_count = CASE WHEN url <> ?1 THEN 0 ELSE fail_count END,
			http_status = CASE WHEN url <> ?1 THEN NULL ELSE http_status END,
			latency_ms = CASE WHEN url <> ?1 THEN NULL ELSE latency_ms END,
			message_i18n = CASE WHEN url <> ?1 THEN NULL ELSE message_i18n END,
			tls_not_after = CASE WHEN url <> ?1 THEN NULL ELSE tls_not_after END,
			tls_issuer = CASE WHEN url <> ?1 THEN NULL ELSE tls_issuer END,
			tls_valid = CASE WHEN url <> ?1 THEN NULL ELSE tls_valid END,
			url = ?1, enabled = ?2, updated_at = ?4
		 WHERE id = ?5`,
		url, enabled, initialProbeStatus(enabled), formatTime(time.Now()), id)
	if err != nil {
		return Probe{}, mapConstraint(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Probe{}, ErrNotFound
	}
	return s.GetProbe(ctx, id)
}

// SetProbeExpectedStatus legt die erwarteten Statuscodes fest ("" = Standard).
// Die Angabe muss vorher geprüft sein (probe.ParseExpected). Ändert sie sich,
// beginnt eine aktive Prüfung von vorn, damit das nächste Ergebnis sofort gilt.
func (s *Store) SetProbeExpectedStatus(ctx context.Context, id int64, spec string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE probes SET
			status = CASE WHEN COALESCE(expected_status, '') <> ?1 AND enabled = 1 THEN ?2 ELSE status END,
			fail_count = CASE WHEN COALESCE(expected_status, '') <> ?1 THEN 0 ELSE fail_count END,
			expected_status = NULLIF(?1, ''), updated_at = ?3
		 WHERE id = ?4`,
		spec, ProbePending, formatTime(time.Now()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func initialProbeStatus(enabled bool) string {
	if enabled {
		return ProbePending
	}
	return ProbePaused
}

// SetRecordProbe legt die Prüfung eines Records an (bzw. passt ihre Adresse an)
// oder entfernt sie (url == "").
func (s *Store) SetRecordProbe(ctx context.Context, recordID int64, url string) error {
	if url == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM probes WHERE record_id = ?`, recordID)
		return err
	}
	now := formatTime(time.Now())
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO probes (record_id, url, status, created_at, updated_at) VALUES (?1, ?2, ?3, ?4, ?4)
		 ON CONFLICT (record_id) DO UPDATE SET
			status = CASE WHEN url <> excluded.url THEN excluded.status ELSE status END,
			fail_count = CASE WHEN url <> excluded.url THEN 0 ELSE fail_count END,
			url = excluded.url, updated_at = excluded.updated_at`,
		recordID, url, ProbePending, now)
	return err
}

// RecordProbe liefert die Prüfung eines Records.
func (s *Store) RecordProbe(ctx context.Context, recordID int64) (Probe, error) {
	p, err := scanProbe(s.db.QueryRowContext(ctx, `SELECT `+probeCols+probeFrom+` WHERE p.record_id = ?`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return Probe{}, ErrNotFound
	}
	return p, err
}

func (s *Store) DeleteProbe(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM probes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetProbe(ctx context.Context, id int64) (Probe, error) {
	p, err := scanProbe(s.db.QueryRowContext(ctx, `SELECT `+probeCols+probeFrom+` WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Probe{}, ErrNotFound
	}
	return p, err
}

func (s *Store) ListProbes(ctx context.Context) ([]Probe, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+probeCols+probeFrom+` ORDER BY p.url, p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Probe{}
	for rows.Next() {
		p, err := scanProbe(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetProbeResult speichert das Ergebnis einer Prüfung. Wurde die Prüfung
// währenddessen geändert (andere Adresse) oder gelöscht, wird nichts
// gespeichert.
func (s *Store) SetProbeResult(ctx context.Context, id int64, url string, r ProbeResult) error {
	var changedAt any
	if r.Changed {
		changedAt = formatTime(r.CheckedAt)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE probes SET status = ?, http_status = ?, latency_ms = ?, fail_count = ?, message_i18n = NULLIF(?, ''),
			tls_not_after = ?, tls_issuer = NULLIF(?, ''), tls_valid = ?, cert_warned_for = ?,
			last_checked_at = ?, last_changed_at = COALESCE(?, last_changed_at)
		 WHERE id = ? AND url = ? AND enabled = 1`,
		r.Status, r.HTTPStatus, r.LatencyMS, r.FailCount, i18n.Encode(r.Message),
		nullTime(r.TLSNotAfter), r.TLSIssuer, r.TLSValid, nullTime(r.CertWarnedFor),
		formatTime(r.CheckedAt), changedAt, id, url)
	return err
}

func scanProbe(sc scanner) (Probe, error) {
	var (
		p                                  Probe
		recordID                           sql.NullInt64
		recordName, msg, issuer, expected  sql.NullString
		httpStatus, latency                sql.NullInt64
		tlsValid                           sql.NullBool
		notAfter, warned, checked, changed sql.NullString
		created, updated                   string
	)
	err := sc.Scan(&p.ID, &recordID, &recordName, &p.URL, &p.Enabled, &p.Status, &httpStatus, &latency,
		&p.FailCount, &msg, &notAfter, &issuer, &tlsValid, &warned, &checked, &changed, &created, &updated, &expected)
	if err != nil {
		return Probe{}, err
	}
	p.ExpectedStatus = expected.String
	if recordID.Valid {
		p.RecordID = &recordID.Int64
	}
	p.RecordName, p.TLSIssuer = recordName.String, issuer.String
	p.MessageMsg = i18n.Decode(msg.String)
	if httpStatus.Valid {
		v := int(httpStatus.Int64)
		p.HTTPStatus = &v
	}
	if latency.Valid {
		v := int(latency.Int64)
		p.LatencyMS = &v
	}
	if tlsValid.Valid {
		p.TLSValid = &tlsValid.Bool
	}
	for _, f := range []struct {
		dst **time.Time
		src sql.NullString
	}{{&p.TLSNotAfter, notAfter}, {&p.CertWarnedFor, warned}, {&p.LastCheckedAt, checked}, {&p.LastChangedAt, changed}} {
		if *f.dst, err = parseNullTime(f.src); err != nil {
			return Probe{}, err
		}
	}
	if p.CreatedAt, err = parseTime(created); err != nil {
		return Probe{}, err
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return Probe{}, err
	}
	return p, nil
}
