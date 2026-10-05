package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

// Speedtest ist das Ergebnis einer Geschwindigkeitsmessung. Ist die Messung
// fehlgeschlagen, fehlen die Messwerte und Error ist gesetzt.
type Speedtest struct {
	ID           int64     `json:"id"`
	StartedAt    time.Time `json:"started_at"`
	DurationMS   int64     `json:"duration_ms"`
	Trigger      string    `json:"trigger"` // manual | scheduled
	DownloadMbps *float64  `json:"download_mbps,omitempty"`
	UploadMbps   *float64  `json:"upload_mbps,omitempty"`
	LatencyMS    *float64  `json:"latency_ms,omitempty"`
	JitterMS     *float64  `json:"jitter_ms,omitempty"`
	Colo         string    `json:"colo,omitempty"`
	City         string    `json:"city,omitempty"`
	Country      string    `json:"country,omitempty"`
	IP           string    `json:"ip,omitempty"`
	// Übertragene Datenmenge (auch die Aufwärmphase).
	DownloadBytes int64 `json:"download_bytes"`
	UploadBytes   int64 `json:"upload_bytes"`
	// Error: von der API gerenderter Text; ErrorMsg: übersetzbare Meldung.
	Error    string   `json:"error,omitempty"`
	ErrorMsg i18n.Msg `json:"-"`
}

const speedtestCols = `id, started_at, duration_ms, trigger, download_mbps, upload_mbps, latency_ms, jitter_ms,
	colo, city, country, ip, download_bytes, upload_bytes, error_i18n`

func (s *Store) InsertSpeedtest(ctx context.Context, t Speedtest) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO speedtests (started_at, duration_ms, trigger, download_mbps, upload_mbps, latency_ms, jitter_ms,
			colo, city, country, ip, download_bytes, upload_bytes, error_i18n)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, NULLIF(?, ''))`,
		formatTime(t.StartedAt), t.DurationMS, t.Trigger, t.DownloadMbps, t.UploadMbps, t.LatencyMS, t.JitterMS,
		t.Colo, t.City, t.Country, t.IP, t.DownloadBytes, t.UploadBytes, i18n.Encode(t.ErrorMsg))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSpeedtests liefert die jüngsten Messungen (neueste zuerst); beforeID > 0
// blättert zurück.
func (s *Store) ListSpeedtests(ctx context.Context, beforeID int64, limit int) ([]Speedtest, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+speedtestCols+` FROM speedtests WHERE (?1 = 0 OR id < ?1) ORDER BY id DESC LIMIT ?2`,
		beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Speedtest{}
	for rows.Next() {
		t, err := scanSpeedtest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// LastSpeedtest liefert die jüngste Messung oder ErrNotFound.
func (s *Store) LastSpeedtest(ctx context.Context) (Speedtest, error) {
	t, err := scanSpeedtest(s.db.QueryRowContext(ctx,
		`SELECT `+speedtestCols+` FROM speedtests ORDER BY id DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return Speedtest{}, ErrNotFound
	}
	return t, err
}

// PruneSpeedtests löscht Messungen, die vor before begonnen haben.
func (s *Store) PruneSpeedtests(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM speedtests WHERE started_at < ?`, formatTime(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanSpeedtest(sc scanner) (Speedtest, error) {
	var (
		t                       Speedtest
		started                 string
		down, up, lat, jit      sql.NullFloat64
		colo, city, country, ip sql.NullString
		errMsg                  sql.NullString
	)
	err := sc.Scan(&t.ID, &started, &t.DurationMS, &t.Trigger, &down, &up, &lat, &jit,
		&colo, &city, &country, &ip, &t.DownloadBytes, &t.UploadBytes, &errMsg)
	if err != nil {
		return Speedtest{}, err
	}
	for _, f := range []struct {
		dst **float64
		src sql.NullFloat64
	}{{&t.DownloadMbps, down}, {&t.UploadMbps, up}, {&t.LatencyMS, lat}, {&t.JitterMS, jit}} {
		if f.src.Valid {
			v := f.src.Float64
			*f.dst = &v
		}
	}
	t.Colo, t.City, t.Country, t.IP = colo.String, city.String, country.String, ip.String
	t.ErrorMsg = i18n.Decode(errMsg.String)
	if t.StartedAt, err = parseTime(started); err != nil {
		return Speedtest{}, err
	}
	return t, nil
}
