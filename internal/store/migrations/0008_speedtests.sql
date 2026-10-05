-- Ergebnisse der Geschwindigkeitsmessung (speed.cloudflare.com). Bei einem
-- Fehler sind die Messwerte NULL und error_i18n gesetzt.
CREATE TABLE speedtests (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at     TEXT NOT NULL,
    duration_ms    INTEGER NOT NULL,
    trigger        TEXT NOT NULL,       -- manual | scheduled
    download_mbps  REAL,
    upload_mbps    REAL,
    latency_ms     REAL,
    jitter_ms      REAL,
    colo           TEXT,                -- Cloudflare-Rechenzentrum (IATA)
    city           TEXT,                -- Standort des Anschlusses laut Cloudflare
    country        TEXT,
    ip             TEXT,                -- öffentliche Adresse der Messung
    download_bytes INTEGER NOT NULL DEFAULT 0,
    upload_bytes   INTEGER NOT NULL DEFAULT 0,
    error_i18n     TEXT
);

CREATE INDEX idx_speedtests_started ON speedtests (started_at);
