CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE ip_changes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    family      TEXT NOT NULL CHECK (family IN ('ipv4', 'ipv6')),
    ip          TEXT NOT NULL,
    previous_ip TEXT,
    detected_at TEXT NOT NULL
);
CREATE INDEX idx_ip_changes_family ON ip_changes (family, id DESC);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_expires ON sessions (expires_at);
