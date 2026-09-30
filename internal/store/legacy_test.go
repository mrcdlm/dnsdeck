package store

import (
	"context"
	"testing"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

func TestUpgradeLegacyMessages(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Zeilen wie von 0.1.0 geschrieben: nur Text, kein message_i18n
	for _, msg := range []string{"bestehenden Eintrag übernommen (Proxy aus, TTL 1 h)", "unbekannter Fehler xyz"} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO update_log (record_name, record_type, trigger, result, old_ip, new_ip, message, created_at)
			VALUES ('a.example.com', 'A', 'manual', 'adopted', '', '', ?, '2026-09-30T10:00:00Z')`, msg); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.UpgradeLegacyMessages(ctx)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n, _ := s.UpgradeLegacyMessages(ctx); n != 0 {
		t.Fatalf("second run converted %d", n)
	}
	l, err := s.ListUpdateLog(ctx, UpdateLogFilter{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range l {
		got[e.Message] = i18n.T(i18n.EN, e.MessageMsg)
	}
	if got["bestehenden Eintrag übernommen (Proxy aus, TTL 1 h)"] != "adopted existing record (proxy off, TTL 1 h)" ||
		got["unbekannter Fehler xyz"] != "" {
		t.Fatalf("%v", got)
	}
}
