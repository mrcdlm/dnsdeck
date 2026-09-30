package store

import (
	"context"
	"fmt"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

// UpgradeLegacyMessages wandelt Meldungen aus Versionen vor 0.2.0 (nur
// deutscher Text) in übersetzbare Meldungen um, soweit sie erkannt werden.
// Unbekannte Texte bleiben unverändert. Liefert die Anzahl umgewandelter Zeilen.
func (s *Store) UpgradeLegacyMessages(ctx context.Context) (int, error) {
	total := 0
	for _, t := range []struct{ table, text, msg string }{
		{"update_log", "message", "message_i18n"},
		{"records", "message", "message_i18n"},
		{"webhooks", "last_error", "last_error_i18n"},
	} {
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf(
			`SELECT id, %s FROM %s WHERE %s IS NULL AND COALESCE(%s, '') != ''`, t.text, t.table, t.msg, t.text))
		if err != nil {
			return total, err
		}
		updates := map[int64]string{}
		for rows.Next() {
			var (
				id   int64
				text string
			)
			if err := rows.Scan(&id, &text); err != nil {
				rows.Close()
				return total, err
			}
			if m, ok := i18n.ParseLegacy(text); ok {
				updates[id] = i18n.Encode(m)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return total, err
		}
		for id, enc := range updates {
			if _, err := s.db.ExecContext(ctx,
				fmt.Sprintf(`UPDATE %s SET %s = ? WHERE id = ?`, t.table, t.msg), enc, id); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
}
