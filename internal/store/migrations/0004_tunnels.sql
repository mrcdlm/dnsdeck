-- Letzter bekannter Stand je Tunnel (Snapshot der Cloudflare-API).
CREATE TABLE tunnels (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    status            TEXT NOT NULL,
    created_at        TEXT NOT NULL,
    conns_active_at   TEXT,
    conns_inactive_at TEXT,
    connections       TEXT NOT NULL DEFAULT '[]', -- JSON
    remote_config     INTEGER NOT NULL DEFAULT 0,
    first_seen_at     TEXT NOT NULL,
    last_seen_at      TEXT NOT NULL,
    removed_at        TEXT                        -- nicht mehr in der API-Liste
);

-- Zeitabschnitte mit gleichem Status. Solange der Status gleich bleibt und
-- regelmäßig abgefragt wird, wächst last_seen_at; Statuswechsel oder Lücken
-- (dnsdeck/Cloudflare nicht erreichbar) beginnen einen neuen Abschnitt.
CREATE TABLE tunnel_status_segments (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tunnel_id    TEXT NOT NULL,
    status       TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);
CREATE INDEX idx_tunnel_segments ON tunnel_status_segments (tunnel_id, started_at);
CREATE INDEX idx_tunnel_segments_last_seen ON tunnel_status_segments (last_seen_at);
