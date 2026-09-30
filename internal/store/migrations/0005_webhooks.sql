-- Generische Webhooks. Geheimnisse stehen hier nie im Klartext, sondern nur
-- als Platzhalter (${WEBHOOK_…} bzw. {{env "WEBHOOK_…"}}); die Werte kommen
-- aus Env-Variablen.
CREATE TABLE webhooks (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    method        TEXT NOT NULL DEFAULT 'POST',
    url           TEXT NOT NULL,
    headers       TEXT NOT NULL DEFAULT '[]', -- JSON [{"name":…,"value":…}]
    content_type  TEXT NOT NULL DEFAULT 'application/json',
    body_template TEXT NOT NULL DEFAULT '',   -- leer = Standard-JSON
    events        TEXT NOT NULL DEFAULT '[]', -- JSON ["ip_change", …]
    last_sent_at  TEXT,
    last_error    TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

-- Globale Ereignisauswahl entfällt (jetzt je Webhook).
DELETE FROM settings WHERE key = 'notify_events';
