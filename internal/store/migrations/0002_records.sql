CREATE TABLE records (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    provider           TEXT NOT NULL DEFAULT 'cloudflare',
    zone_id            TEXT NOT NULL,
    zone_name          TEXT NOT NULL,
    name               TEXT NOT NULL,             -- FQDN, klein geschrieben
    type               TEXT NOT NULL CHECK (type IN ('A', 'AAAA')),
    proxied            INTEGER NOT NULL DEFAULT 0,
    ttl                INTEGER NOT NULL DEFAULT 1, -- 1 = automatisch
    enabled            INTEGER NOT NULL DEFAULT 1,
    provider_record_id TEXT,
    current_ip         TEXT,                      -- zuletzt beim Provider gesehener Inhalt
    status             TEXT NOT NULL DEFAULT 'pending',
    message            TEXT,
    last_checked_at    TEXT,
    last_changed_at    TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    UNIQUE (name, type)
);

CREATE TABLE update_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    record_id   INTEGER REFERENCES records (id) ON DELETE SET NULL,
    record_name TEXT NOT NULL,
    record_type TEXT NOT NULL,
    trigger     TEXT NOT NULL,  -- scheduled | manual | record_saved
    result      TEXT NOT NULL,  -- created | updated | error
    old_ip      TEXT,
    new_ip      TEXT,
    message     TEXT,
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_update_log_record ON update_log (record_id, id DESC);
