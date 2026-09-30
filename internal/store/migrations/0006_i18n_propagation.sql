-- Übersetzbare Meldungen: Code + Parameter als JSON (i18n.Msg). Die alten
-- Textspalten bleiben für bestehende Einträge erhalten.
ALTER TABLE records    ADD COLUMN message_i18n TEXT;
ALTER TABLE update_log ADD COLUMN message_i18n TEXT;
ALTER TABLE webhooks   ADD COLUMN last_error_i18n TEXT;

-- Ergebnis der letzten DNS-Verbreitungsprüfung (JSON, siehe dnscheck.Result).
ALTER TABLE records ADD COLUMN propagation TEXT;
