-- Proxy/TTL wurden in dnsdeck geändert und müssen noch zu Cloudflare
-- übertragen werden. Ohne diese Markierung übernimmt dnsdeck die Werte von
-- Cloudflare (Cloudflare hat bei Proxy/TTL das letzte Wort).
ALTER TABLE records ADD COLUMN settings_pending INTEGER NOT NULL DEFAULT 0;
