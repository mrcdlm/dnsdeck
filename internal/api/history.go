package api

import (
	"net/http"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

// Verlauf: alle Listen liefern die neuesten Einträge zuerst; mit before_id
// bzw. before (Zeitpunkt) werden ältere nachgeladen.

func (s *Server) handleIPHistory(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryInt(w, r, "limit", 50, 500)
	if !ok {
		return
	}
	before, ok := queryInt(w, r, "before_id", 0, 1<<62)
	if !ok {
		return
	}
	family := r.URL.Query().Get("family")
	if family != "" && family != "ipv4" && family != "ipv6" {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.family_invalid"))
		return
	}
	list, err := s.store.ListIPChanges(r.Context(), store.IPChangeFilter{Family: family, BeforeID: int64(before)}, limit)
	if err != nil {
		s.internalError(w, r, "IP-Verlauf lesen", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

var updateResults = map[string]bool{
	store.ResultCreated: true, store.ResultAdopted: true, store.ResultUpdated: true,
	store.ResultRecovered: true, store.ResultError: true,
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
	before, ok := queryInt(w, r, "before_id", 0, 1<<62)
	if !ok {
		return
	}
	result := r.URL.Query().Get("result")
	if result != "" && !updateResults[result] {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.invalid_param", "name", "result"))
		return
	}
	entries, err := s.store.ListUpdateLog(r.Context(),
		store.UpdateLogFilter{RecordID: int64(recID), Result: result, BeforeID: int64(before)}, limit)
	if err != nil {
		s.internalError(w, r, "Update-Log lesen", err)
		return
	}
	lang := i18n.FromRequest(r)
	for i := range entries {
		if !entries[i].MessageMsg.IsZero() {
			entries[i].Message = i18n.T(lang, entries[i].MessageMsg)
		}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleTunnelHistory(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryInt(w, r, "limit", 50, 500)
	if !ok {
		return
	}
	f := store.TunnelChangeFilter{TunnelID: r.URL.Query().Get("tunnel_id")}
	if v := r.URL.Query().Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeMsg(w, r, http.StatusBadRequest, i18n.M("api.before_invalid"))
			return
		}
		f.Before = t
	}
	list, err := s.store.TunnelStatusChanges(r.Context(), f, limit)
	if err != nil {
		s.internalError(w, r, "Tunnel-Verlauf lesen", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
