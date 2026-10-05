package api

import (
	"net/http"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

// SpeedtestRunner startet Geschwindigkeitsmessungen im Hintergrund.
type SpeedtestRunner interface {
	Start(trigger string) bool
	Status() (running bool, since time.Time)
}

type speedtestDTO struct {
	// Available: Messung möglich (sonst nur Anzeige gespeicherter Ergebnisse).
	Available bool       `json:"available"`
	Running   bool       `json:"running"`
	RunningAt *time.Time `json:"running_since,omitempty"`
	// IntervalSeconds: Abstand geplanter Messungen; 0 = nur manuell.
	IntervalSeconds int               `json:"interval_seconds"`
	Results         []store.Speedtest `json:"results"` // neueste zuerst
}

// handleSpeedtest liefert Status und die jüngsten Messungen (?limit, ?before_id).
func (s *Server) handleSpeedtest(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryInt(w, r, "limit", 50, 500)
	if !ok {
		return
	}
	before, ok := queryInt(w, r, "before_id", 0, 1<<62)
	if !ok {
		return
	}
	results, err := s.store.ListSpeedtests(r.Context(), int64(before), limit)
	if err != nil {
		s.internalError(w, r, "Speedtests lesen", err)
		return
	}
	lang := i18n.FromRequest(r)
	for i := range results {
		results[i].Error = i18n.T(lang, results[i].ErrorMsg)
	}
	out := speedtestDTO{Available: s.speedtest != nil, Results: results}
	if s.speedtest != nil {
		if running, since := s.speedtest.Status(); running {
			out.Running, out.RunningAt = true, &since
		}
	}
	if s.settings != nil {
		out.IntervalSeconds = int(s.settings.Get().SpeedtestInterval.Seconds())
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRunSpeedtest startet eine Messung. Sie dauert rund 20 Sekunden und
// läuft im Hintergrund; das Ergebnis kommt per Live-Update (Thema speedtest).
func (s *Server) handleRunSpeedtest(w http.ResponseWriter, r *http.Request) {
	if s.speedtest == nil {
		writeMsg(w, r, http.StatusServiceUnavailable, i18n.M("api.not_found"))
		return
	}
	if !s.speedtest.Start(store.TriggerManual) {
		writeMsg(w, r, http.StatusConflict, i18n.M("speedtest.running"))
		return
	}
	s.log.Info("Speedtest gestartet")
	_, since := s.speedtest.Status()
	writeJSON(w, http.StatusAccepted, map[string]any{"running": true, "running_since": since})
}
