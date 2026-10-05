-- Erwarteter HTTP-Status einer Prüfung, z. B. "200", "200,204", "2xx", "200-399";
-- NULL = jede Antwort unter 500 gilt als erreichbar.
ALTER TABLE probes ADD COLUMN expected_status TEXT;

-- Statusverlauf je Prüfung (wie tunnel_status_segments): gleicher Status bei
-- regelmäßiger Prüfung verlängert den Abschnitt, Wechsel oder Lücken beginnen einen neuen.
CREATE TABLE probe_status_segments (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    probe_id     INTEGER NOT NULL REFERENCES probes(id) ON DELETE CASCADE,
    status       TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);
CREATE INDEX idx_probe_segments ON probe_status_segments (probe_id, started_at);
CREATE INDEX idx_probe_segments_last_seen ON probe_status_segments (last_seen_at);
