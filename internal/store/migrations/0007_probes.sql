-- Erreichbarkeitsprüfungen (HTTP/HTTPS) eigener Dienste. record_id gesetzt =
-- Prüfung von https://<record-name>/, die mit dem Record angelegt und gelöscht wird.
CREATE TABLE probes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    record_id       INTEGER UNIQUE REFERENCES records(id) ON DELETE CASCADE,
    url             TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'pending',
    http_status     INTEGER,
    latency_ms      INTEGER,
    fail_count      INTEGER NOT NULL DEFAULT 0, -- Fehlschläge in Folge (Entprellung)
    message_i18n    TEXT,
    tls_not_after   TEXT,
    tls_issuer      TEXT,
    tls_valid       INTEGER,                    -- NULL = kein TLS
    cert_warned_for TEXT,                       -- NotAfter des Zertifikats, vor dem zuletzt gewarnt wurde
    last_checked_at TEXT,
    last_changed_at TEXT,                       -- letzter Statuswechsel
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

-- Eigene Prüfungen nicht doppelt anlegen (Record-Prüfungen sind über record_id eindeutig).
CREATE UNIQUE INDEX idx_probes_url ON probes (url) WHERE record_id IS NULL;
