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

	"github.com/mrcdlm/dnsdeck/internal/providers"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

type recordStore interface {
	ListRecords(ctx context.Context) ([]store.Record, error)
	GetRecord(ctx context.Context, id int64) (store.Record, error)
	CreateRecord(ctx context.Context, r store.Record) (store.Record, error)
	UpdateRecordSettings(ctx context.Context, r store.Record) (store.Record, error)
	DeleteRecord(ctx context.Context, id int64) error
	ListUpdateLog(ctx context.Context, recordID int64, limit int) ([]store.UpdateLogEntry, error)
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
		s.writeProviderError(w, err)
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

func (s *Server) writeProviderError(w http.ResponseWriter, err error) {
	if errors.Is(err, providers.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, "Cloudflare ist nicht konfiguriert (CF_API_TOKEN fehlt)")
		return
	}
	s.log.Error("Cloudflare-Anfrage fehlgeschlagen", "err", err)
	writeError(w, http.StatusBadGateway, err.Error())
}

func (s *Server) handleListRecords(w http.ResponseWriter, r *http.Request) {
	recs, err := s.store.ListRecords(r.Context())
	if err != nil {
		s.internalError(w, "Records lesen", err)
		return
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
func (s *Server) toRecord(ctx context.Context, in recordInput) (store.Record, int, string) {
	zones, err := s.listZones(ctx)
	if errors.Is(err, providers.ErrNotConfigured) {
		return store.Record{}, http.StatusServiceUnavailable, "Cloudflare ist nicht konfiguriert (CF_API_TOKEN fehlt)"
	}
	if err != nil {
		return store.Record{}, http.StatusBadGateway, "Zonen konnten nicht geladen werden: " + err.Error()
	}
	var zone *providers.Zone
	for i := range zones {
		if zones[i].ID == in.ZoneID {
			zone = &zones[i]
		}
	}
	if zone == nil {
		return store.Record{}, http.StatusBadRequest, "Zone unbekannt"
	}

	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(in.Name)), ".")
	zoneName := strings.ToLower(zone.Name)
	if name == "" || name == "@" {
		name = zoneName
	}
	if name != zoneName && !strings.HasSuffix(name, "."+zoneName) {
		return store.Record{}, http.StatusBadRequest, "Name muss in der Zone " + zoneName + " liegen"
	}
	if len(name) > 253 {
		return store.Record{}, http.StatusBadRequest, "Name zu lang"
	}
	for _, l := range strings.Split(name, ".") {
		if !labelRe.MatchString(l) {
			return store.Record{}, http.StatusBadRequest, "ungültiger Name: " + name
		}
	}

	if in.Type != "A" && in.Type != "AAAA" {
		return store.Record{}, http.StatusBadRequest, "Typ muss A oder AAAA sein"
	}
	ttl := in.TTL
	if in.Proxied || ttl == 0 {
		ttl = 1 // Cloudflare: proxied Einträge haben immer TTL "automatisch"
	}
	if ttl != 1 && (ttl < 60 || ttl > 86400) {
		return store.Record{}, http.StatusBadRequest, "TTL muss 1 (automatisch) oder 60–86400 Sekunden sein"
	}
	enabled := in.Enabled == nil || *in.Enabled

	return store.Record{ZoneID: zone.ID, ZoneName: zone.Name, Name: name, Type: in.Type,
		Proxied: in.Proxied, TTL: ttl, Enabled: enabled}, 0, ""
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "ungültige Anfrage")
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
		writeError(w, status, msg)
		return
	}
	created, err := s.store.CreateRecord(r.Context(), rec)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, rec.Type+"-Eintrag für "+rec.Name+" wird bereits verwaltet")
		return
	}
	if err != nil {
		s.internalError(w, "Record anlegen", err)
		return
	}
	writeJSON(w, http.StatusCreated, s.syncAfterSave(r, created))
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
		writeError(w, status, msg)
		return
	}
	rec.ID = id
	updated, err := s.store.UpdateRecordSettings(r.Context(), rec)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Record nicht gefunden")
		return
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, rec.Type+"-Eintrag für "+rec.Name+" wird bereits verwaltet")
		return
	case err != nil:
		s.internalError(w, "Record ändern", err)
		return
	}
	writeJSON(w, http.StatusOK, s.syncAfterSave(r, updated))
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
		writeError(w, http.StatusNotFound, "Record nicht gefunden")
		return
	}
	if err != nil {
		s.internalError(w, "Record löschen", err)
		return
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
		writeError(w, http.StatusNotFound, "Record nicht gefunden")
		return
	}
	if err != nil {
		s.internalError(w, "Record abgleichen", err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := detached(r)
	defer cancel()
	if _, err := s.ddns.RunCycle(ctx, store.TriggerManual); err != nil {
		s.internalError(w, "Abgleich", err)
		return
	}
	s.handleListRecords(w, r)
}

func (s *Server) handleUpdateLog(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryInt(w, r, "limit", 50, 500)
	if !ok {
		return
	}
	recID, ok := queryInt(w, r, "record_id", 0, 1<<62)
	if !ok {
		return
	}
	entries, err := s.store.ListUpdateLog(r.Context(), int64(recID), limit)
	if err != nil {
		s.internalError(w, "Update-Log lesen", err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func recordID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "ungültige ID")
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
		writeError(w, http.StatusBadRequest, key+" ungültig")
		return 0, false
	}
	return min(n, max), true
}

func (s *Server) internalError(w http.ResponseWriter, what string, err error) {
	s.log.Error(what+" fehlgeschlagen", "err", err)
	writeError(w, http.StatusInternalServerError, "interner Fehler")
}
