package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/probe"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

type probeStore interface {
	ListProbes(ctx context.Context) ([]store.Probe, error)
	GetProbe(ctx context.Context, id int64) (store.Probe, error)
	CreateProbe(ctx context.Context, url string, enabled bool) (store.Probe, error)
	UpdateProbe(ctx context.Context, id int64, url string, enabled bool) (store.Probe, error)
	DeleteProbe(ctx context.Context, id int64) error
	SetRecordProbe(ctx context.Context, recordID int64, url string) error
	RecordProbe(ctx context.Context, recordID int64) (store.Probe, error)
}

// ProbeRunner führt eine Erreichbarkeitsprüfung sofort aus.
type ProbeRunner interface {
	Run(ctx context.Context, id int64) (store.Probe, error)
}

func localizeProbe(p store.Probe, lang i18n.Lang) store.Probe {
	p.Message = i18n.T(lang, p.MessageMsg)
	return p
}

func (s *Server) handleListProbes(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListProbes(r.Context())
	if err != nil {
		s.internalError(w, r, "Prüfungen lesen", err)
		return
	}
	lang := i18n.FromRequest(r)
	for i := range list {
		list[i] = localizeProbe(list[i], lang)
	}
	writeJSON(w, http.StatusOK, list)
}

type probeInput struct {
	URL     *string `json:"url"`
	Enabled *bool   `json:"enabled"`
}

func (s *Server) handleCreateProbe(w http.ResponseWriter, r *http.Request) {
	var in probeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.URL == nil {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("probe.url_invalid"))
		return
	}
	u, err := probe.ValidateURL(*in.URL)
	if err != nil {
		writeMsg(w, r, http.StatusBadRequest, i18n.FromError(err))
		return
	}
	p, err := s.store.CreateProbe(r.Context(), u, in.Enabled == nil || *in.Enabled)
	if errors.Is(err, store.ErrConflict) {
		writeMsg(w, r, http.StatusConflict, i18n.M("probe.conflict", "url", u))
		return
	}
	if err != nil {
		s.internalError(w, r, "Prüfung anlegen", err)
		return
	}
	s.log.Info("Erreichbarkeitsprüfung angelegt", "url", p.URL)
	writeJSON(w, http.StatusCreated, localizeProbe(s.runProbe(r, p), i18n.FromRequest(r)))
}

func (s *Server) handleUpdateProbe(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	var in probeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	old, err := s.store.GetProbe(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("probe.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Prüfung lesen", err)
		return
	}
	u, enabled := old.URL, old.Enabled
	if in.URL != nil {
		if u, err = probe.ValidateURL(*in.URL); err != nil {
			writeMsg(w, r, http.StatusBadRequest, i18n.FromError(err))
			return
		}
		if old.RecordID != nil && u != old.URL {
			writeMsg(w, r, http.StatusBadRequest, i18n.M("probe.record_url"))
			return
		}
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	p, err := s.store.UpdateProbe(r.Context(), id, u, enabled)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeMsg(w, r, http.StatusNotFound, i18n.M("probe.not_found"))
		return
	case errors.Is(err, store.ErrConflict):
		writeMsg(w, r, http.StatusConflict, i18n.M("probe.conflict", "url", u))
		return
	case err != nil:
		s.internalError(w, r, "Prüfung ändern", err)
		return
	}
	if p.Status == store.ProbePending { // neu begonnen (Adresse geändert oder wieder aktiviert)
		p = s.runProbe(r, p)
	} else {
		events.Publish(s.events, events.TopicProbes)
	}
	writeJSON(w, http.StatusOK, localizeProbe(p, i18n.FromRequest(r)))
}

func (s *Server) handleDeleteProbe(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteProbe(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("probe.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Prüfung löschen", err)
		return
	}
	events.Publish(s.events, events.TopicProbes)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRunProbe(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	p, err := s.store.GetProbe(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("probe.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Prüfung lesen", err)
		return
	}
	writeJSON(w, http.StatusOK, localizeProbe(s.runProbe(r, p), i18n.FromRequest(r)))
}

// runProbe prüft sofort, damit der Benutzer das Ergebnis direkt sieht.
// Fehler werden geloggt; zurück kommt dann der bisherige Stand.
func (s *Server) runProbe(r *http.Request, p store.Probe) store.Probe {
	if s.probes == nil || !p.Enabled {
		events.Publish(s.events, events.TopicProbes)
		return p
	}
	ctx, cancel := detached(r)
	defer cancel()
	checked, err := s.probes.Run(ctx, p.ID)
	if err != nil {
		s.log.Error("Erreichbarkeit prüfen fehlgeschlagen", "url", p.URL, "err", err)
		return p
	}
	return checked
}

// recordProbeURL ist die geprüfte Adresse eines Records.
func recordProbeURL(rec store.Record) string { return "https://" + rec.Name + "/" }

// syncRecordProbe legt die Prüfung eines Records an bzw. entfernt sie
// (want != nil) oder führt nur ihre Adresse nach einer Namensänderung nach.
// Die erste Prüfung läuft im Hintergrund, um das Speichern nicht aufzuhalten.
func (s *Server) syncRecordProbe(ctx context.Context, rec store.Record, want *bool) {
	existing, err := s.store.RecordProbe(ctx, rec.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.log.Error("Prüfung des Records lesen fehlgeschlagen", "record", rec.Name, "err", err)
		return
	}
	has := err == nil
	enable := has
	if want != nil {
		enable = *want
	}
	switch {
	case !enable && !has:
		return
	case !enable:
		err = s.store.SetRecordProbe(ctx, rec.ID, "")
	case has && existing.URL == recordProbeURL(rec):
		return
	default:
		err = s.store.SetRecordProbe(ctx, rec.ID, recordProbeURL(rec))
	}
	if err != nil {
		s.log.Error("Prüfung des Records speichern fehlgeschlagen", "record", rec.Name, "err", err)
		return
	}
	events.Publish(s.events, events.TopicProbes)
	if !enable || s.probes == nil {
		return
	}
	p, err := s.store.RecordProbe(ctx, rec.ID)
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), syncTimeout)
		defer cancel()
		if _, err := s.probes.Run(ctx, p.ID); err != nil {
			s.log.Error("Erreichbarkeit prüfen fehlgeschlagen", "url", p.URL, "err", err)
		}
	}()
}
