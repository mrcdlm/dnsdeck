package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/providers"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

type recordStore interface {
	ListRecords(ctx context.Context) ([]store.Record, error)
	GetRecord(ctx context.Context, id int64) (store.Record, error)
	CreateRecord(ctx context.Context, r store.Record) (store.Record, error)
	UpdateRecordSettings(ctx context.Context, r store.Record) (store.Record, error)
	DeleteRecord(ctx context.Context, id int64) error
	ListUpdateLog(ctx context.Context, f store.UpdateLogFilter, limit int) ([]store.UpdateLogEntry, error)
}

// Abgleich nach dem Speichern bzw. manuell: großzügiges Zeitlimit, und auch
// dann zu Ende führen, wenn der Browser die Verbindung schließt.
const syncTimeout = 60 * time.Second

func detached(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), syncTimeout)
}

func (s *Server) handleZones(w http.ResponseWriter, r *http.Request) {
	zones, err := s.listZones(r.Context())
	if err != nil {
		s.writeProviderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, zones)
}

func (s *Server) listZones(ctx context.Context) ([]providers.Zone, error) {
	if s.zones == nil {
		return nil, providers.ErrNotConfigured
	}
	zones, err := s.zones.ListZones(ctx)
	if zones == nil {
		zones = []providers.Zone{}
	}
	return zones, err
}

func (s *Server) writeProviderError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, providers.ErrNotConfigured) {
		writeMsg(w, r, http.StatusServiceUnavailable, i18n.M("cf.not_configured"))
		return
	}
	s.log.Error("Cloudflare-Anfrage fehlgeschlagen", "err", err)
	writeMsg(w, r, http.StatusBadGateway, i18n.FromError(err))
}

// localizeRecord setzt Message in der Sprache der Anfrage (Alttexte bleiben).
func localizeRecord(rec store.Record, lang i18n.Lang) store.Record {
	if !rec.MessageMsg.IsZero() {
		rec.Message = i18n.T(lang, rec.MessageMsg)
	}
	return rec
}

func (s *Server) handleListRecords(w http.ResponseWriter, r *http.Request) {
	recs, err := s.store.ListRecords(r.Context())
	if err != nil {
		s.internalError(w, r, "Records lesen", err)
		return
	}
	lang := i18n.FromRequest(r)
	for i := range recs {
		recs[i] = localizeRecord(recs[i], lang)
	}
	writeJSON(w, http.StatusOK, recs)
}

type recordInput struct {
	ZoneID  string `json:"zone_id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
	Enabled *bool  `json:"enabled"`
}

var labelRe = regexp.MustCompile(`^[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?$`)

// toRecord prüft die Eingabe und ergänzt den Zonennamen. Liefert eine für den
// Benutzer verständliche Fehlermeldung.
func (s *Server) toRecord(ctx context.Context, in recordInput) (store.Record, int, i18n.Msg) {
	zones, err := s.listZones(ctx)
	if errors.Is(err, providers.ErrNotConfigured) {
		return store.Record{}, http.StatusServiceUnavailable, i18n.M("cf.not_configured")
	}
	if err != nil {
		return store.Record{}, http.StatusBadGateway, i18n.M("cf.zones_failed", "detail", i18n.Nest(i18n.FromError(err)))
	}
	var zone *providers.Zone
	for i := range zones {
		if zones[i].ID == in.ZoneID {
			zone = &zones[i]
		}
	}
	if zone == nil {
		return store.Record{}, http.StatusBadRequest, i18n.M("record.zone_unknown")
	}

	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(in.Name)), ".")
	zoneName := strings.ToLower(zone.Name)
	if name == "" || name == "@" {
		name = zoneName
	}
	if name != zoneName && !strings.HasSuffix(name, "."+zoneName) {
		return store.Record{}, http.StatusBadRequest, i18n.M("record.name_outside_zone", "zone", zoneName)
	}
	if len(name) > 253 {
		return store.Record{}, http.StatusBadRequest, i18n.M("record.name_too_long")
	}
	for _, l := range strings.Split(name, ".") {
		if !labelRe.MatchString(l) {
			return store.Record{}, http.StatusBadRequest, i18n.M("record.name_invalid", "name", name)
		}
	}

	if in.Type != "A" && in.Type != "AAAA" {
		return store.Record{}, http.StatusBadRequest, i18n.M("record.type_invalid")
	}
	ttl := in.TTL
	if in.Proxied || ttl == 0 {
		ttl = 1 // Cloudflare: proxied Einträge haben immer TTL "automatisch"
	}
	if ttl != 1 && (ttl < 60 || ttl > 86400) {
		return store.Record{}, http.StatusBadRequest, i18n.M("record.ttl_invalid")
	}
	enabled := in.Enabled == nil || *in.Enabled

	return store.Record{ZoneID: zone.ID, ZoneName: zone.Name, Name: name, Type: in.Type,
		Proxied: in.Proxied, TTL: ttl, Enabled: enabled}, 0, i18n.Msg{}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.bad_request"))
		return false
	}
	return true
}

func (s *Server) handleCreateRecord(w http.ResponseWriter, r *http.Request) {
	var in recordInput
	if !decodeJSON(w, r, &in) {
		return
	}
	rec, status, msg := s.toRecord(r.Context(), in)
	if status != 0 {
		writeMsg(w, r, status, msg)
		return
	}
	created, err := s.store.CreateRecord(r.Context(), rec)
	if errors.Is(err, store.ErrConflict) {
		writeMsg(w, r, http.StatusConflict, i18n.M("record.conflict", "type", rec.Type, "name", rec.Name))
		return
	}
	if err != nil {
		s.internalError(w, r, "Record anlegen", err)
		return
	}
	writeJSON(w, http.StatusCreated, localizeRecord(s.syncAfterSave(r, created), i18n.FromRequest(r)))
}

func (s *Server) handleUpdateRecord(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	var in recordInput
	if !decodeJSON(w, r, &in) {
		return
	}
	rec, status, msg := s.toRecord(r.Context(), in)
	if status != 0 {
		writeMsg(w, r, status, msg)
		return
	}
	rec.ID = id
	updated, err := s.store.UpdateRecordSettings(r.Context(), rec)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeMsg(w, r, http.StatusNotFound, i18n.M("record.not_found"))
		return
	case errors.Is(err, store.ErrConflict):
		writeMsg(w, r, http.StatusConflict, i18n.M("record.conflict", "type", rec.Type, "name", rec.Name))
		return
	case err != nil:
		s.internalError(w, r, "Record ändern", err)
		return
	}
	writeJSON(w, http.StatusOK, localizeRecord(s.syncAfterSave(r, updated), i18n.FromRequest(r)))
}

// syncAfterSave gleicht einen gespeicherten Record sofort ab, damit der
// Benutzer das Ergebnis direkt sieht. Fehler stehen im Record-Status.
func (s *Server) syncAfterSave(r *http.Request, rec store.Record) store.Record {
	ctx, cancel := detached(r)
	defer cancel()
	synced, err := s.ddns.SyncRecord(ctx, rec.ID, store.TriggerRecordSaved)
	if err != nil {
		s.log.Error("Abgleich nach Speichern fehlgeschlagen", "record", rec.Name, "err", err)
		return rec
	}
	return synced
}

func (s *Server) handleDeleteRecord(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteRecord(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("record.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Record löschen", err)
		return
	}
	if s.events != nil {
		s.events.Publish(events.TopicRecords)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSyncRecord(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(w, r)
	if !ok {
		return
	}
	ctx, cancel := detached(r)
	defer cancel()
	rec, err := s.ddns.SyncRecord(ctx, id, store.TriggerManual)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("record.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Record abgleichen", err)
		return
	}
	writeJSON(w, http.StatusOK, localizeRecord(rec, i18n.FromRequest(r)))
}

func (s *Server) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := detached(r)
	defer cancel()
	if _, err := s.ddns.RunCycle(ctx, store.TriggerManual); err != nil {
		s.internalError(w, r, "Abgleich", err)
		return
	}
	s.handleListRecords(w, r)
}

func recordID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.invalid_id"))
		return 0, false
	}
	return id, true
}

func queryInt(w http.ResponseWriter, r *http.Request, key string, def, max int) (int, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.invalid_param", "name", key))
		return 0, false
	}
	return min(n, max), true
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, what string, err error) {
	s.log.Error(what+" fehlgeschlagen", "err", err)
	writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.internal"))
}
