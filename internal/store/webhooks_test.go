package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

func TestWebhooksCRUD(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	w, err := s.CreateWebhook(ctx, Webhook{Name: "Telegram", Enabled: true, Method: "POST",
		URL: "https://api.telegram.org/bot${WEBHOOK_TG}/sendMessage", ContentType: "application/json",
		Headers: []Header{{"X-A", "1"}}, Events: []string{"tunnel_status"}})
	if err != nil {
		t.Fatal(err)
	}
	if w.ID == 0 || len(w.Headers) != 1 || w.Events[0] != "tunnel_status" || w.LastSentAt != nil {
		t.Fatalf("create: %+v", w)
	}

	now := time.Now()
	s.SetWebhookResult(ctx, w.ID, now, errors.New("HTTP 500"))
	w, _ = s.GetWebhook(ctx, w.ID)
	if i18n.T(i18n.EN, w.LastErrorMsg) != "HTTP 500" || w.LastSentAt != nil {
		t.Fatalf("Fehler: %+v", w)
	}
	s.SetWebhookResult(ctx, w.ID, now, nil)
	w, _ = s.GetWebhook(ctx, w.ID)
	if !w.LastErrorMsg.IsZero() || w.LastSentAt == nil {
		t.Fatalf("Erfolg: %+v", w)
	}

	// Nur Name/Aktiv/Ereignisse geändert → Zustellstatus bleibt
	s.SetWebhookResult(ctx, w.ID, now, errors.New("HTTP 502"))
	w.Enabled = false
	w.Events = []string{"ip_change"}
	w, _ = s.UpdateWebhook(ctx, w)
	if i18n.T(i18n.EN, w.LastErrorMsg) != "HTTP 502" || w.LastSentAt == nil {
		t.Fatalf("Status verloren: %+v", w)
	}

	// URL/Header/Body geändert → Status zurückgesetzt
	w.Name, w.Headers, w.Events = "Neu", nil, nil
	w, err = s.UpdateWebhook(ctx, w)
	if err != nil || w.Name != "Neu" || w.Headers == nil || len(w.Events) != 0 || !w.LastErrorMsg.IsZero() || w.LastSentAt != nil {
		t.Fatalf("update: %v %+v", err, w)
	}
	if list, _ := s.ListWebhooks(ctx); len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
	if err := s.DeleteWebhook(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetWebhook(ctx, w.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nach Löschen: %v", err)
	}
	if _, err := s.UpdateWebhook(ctx, w); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update gelöscht: %v", err)
	}
}
