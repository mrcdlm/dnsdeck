package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

type webhookStore interface {
	ListWebhooks(ctx context.Context) ([]store.Webhook, error)
	GetWebhook(ctx context.Context, id int64) (store.Webhook, error)
	CreateWebhook(ctx context.Context, w store.Webhook) (store.Webhook, error)
	UpdateWebhook(ctx context.Context, w store.Webhook) (store.Webhook, error)
	DeleteWebhook(ctx context.Context, id int64) error
}

type webhookTester interface {
	SendTest(ctx context.Context, id int64) (notify.TestResult, error)
}

// webhookDTO ergänzt die gespeicherte Konfiguration um verwendete, aber nicht
// gesetzte Env-Variablen (nur Namen, nie Werte).
type webhookDTO struct {
	store.Webhook
	MissingEnv []string `json:"missing_env"`
}

func (s *Server) toWebhookDTO(w store.Webhook) webhookDTO {
	missing := []string{}
	if s.webhookEnv != nil {
		if m := notify.MissingEnv(w, s.webhookEnv); m != nil {
			missing = m
		}
	}
	return webhookDTO{Webhook: w, MissingEnv: missing}
}

type webhookInput struct {
	Name         string         `json:"name"`
	Enabled      *bool          `json:"enabled"`
	Method       string         `json:"method"`
	URL          string         `json:"url"`
	Headers      []store.Header `json:"headers"`
	ContentType  string         `json:"content_type"`
	BodyTemplate string         `json:"body_template"`
	Events       []string       `json:"events"`
}

func (in webhookInput) toWebhook() store.Webhook {
	w := store.Webhook{
		Name: strings.TrimSpace(in.Name), Enabled: in.Enabled == nil || *in.Enabled,
		Method: strings.ToUpper(strings.TrimSpace(in.Method)), URL: strings.TrimSpace(in.URL),
		ContentType: strings.TrimSpace(in.ContentType), BodyTemplate: in.BodyTemplate,
		Headers: []store.Header{}, Events: []string{},
	}
	if w.Method == "" {
		w.Method = http.MethodPost
	}
	for _, h := range in.Headers {
		if n := strings.TrimSpace(h.Name); n != "" {
			w.Headers = append(w.Headers, store.Header{Name: n, Value: strings.TrimSpace(h.Value)})
		}
	}
	// Reihenfolge wie in notify.EventTypes, doppelte entfernen
	for _, t := range append(slices.Clone(notify.EventTypes), in.Events...) {
		if slices.Contains(in.Events, t) && !slices.Contains(w.Events, t) {
			w.Events = append(w.Events, t)
		}
	}
	return w
}

func (s *Server) webhookChanged() {
	if s.events != nil {
		s.events.Publish(events.TopicWebhooks)
	}
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListWebhooks(r.Context())
	if err != nil {
		s.internalError(w, "Webhooks lesen", err)
		return
	}
	out := make([]webhookDTO, len(list))
	for i, wh := range list {
		out[i] = s.toWebhookDTO(wh)
	}
	writeJSON(w, http.StatusOK, out)
}

// decodeWebhook liest und prüft die Eingabe; bei Fehlern ist die Antwort geschrieben.
func decodeWebhook(w http.ResponseWriter, r *http.Request) (store.Webhook, bool) {
	var in webhookInput
	if !decodeJSON(w, r, &in) {
		return store.Webhook{}, false
	}
	wh := in.toWebhook()
	if err := notify.Validate(wh); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return store.Webhook{}, false
	}
	return wh, true
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	wh, ok := decodeWebhook(w, r)
	if !ok {
		return
	}
	created, err := s.store.CreateWebhook(r.Context(), wh)
	if err != nil {
		s.internalError(w, "Webhook anlegen", err)
		return
	}
	s.log.Info("Webhook angelegt", "webhook", created.Name)
	s.webhookChanged()
	writeJSON(w, http.StatusCreated, s.toWebhookDTO(created))
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	wh, ok := decodeWebhook(w, r)
	if !ok {
		return
	}
	wh.ID = id
	updated, err := s.store.UpdateWebhook(r.Context(), wh)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Webhook nicht gefunden")
		return
	}
	if err != nil {
		s.internalError(w, "Webhook ändern", err)
		return
	}
	s.webhookChanged()
	writeJSON(w, http.StatusOK, s.toWebhookDTO(updated))
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteWebhook(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Webhook nicht gefunden")
		return
	}
	if err != nil {
		s.internalError(w, "Webhook löschen", err)
		return
	}
	s.webhookChanged()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	if s.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "Webhooks nicht verfügbar")
		return
	}
	ctx, cancel := detached(r)
	defer cancel()
	res, err := s.webhooks.SendTest(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Webhook nicht gefunden")
		return
	}
	if err != nil {
		s.internalError(w, "Webhook testen", err)
		return
	}
	s.webhookChanged()
	writeJSON(w, http.StatusOK, res)
}

// handlePreviewWebhook rendert eine (auch ungespeicherte) Konfiguration mit
// einem Beispielereignis. Geheimnisse bleiben Platzhalter.
func (s *Server) handlePreviewWebhook(w http.ResponseWriter, r *http.Request) {
	var in struct {
		webhookInput
		EventType string `json:"event_type"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	req, err := notify.Render(in.toWebhook(), notify.SampleEvent(in.EventType), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}
