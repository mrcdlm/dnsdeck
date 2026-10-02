package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/config"
	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

type settingsService interface {
	Get() config.Settings
	Update(ctx context.Context, s config.Settings) (config.Settings, error)
	SourceNames() []string
}

// Info sind nicht geheime Angaben zur Installation.
type Info struct {
	Version      string `json:"version"`
	CFTokenSet   bool   `json:"cf_token_set"`
	CFAccountSet bool   `json:"cf_account_set"`
	DataDir      string `json:"data_dir"`
	// DNSCheck: Verbreitungsprüfung aktiv (DNSCHECK_RESOLVERS ≠ off)
	DNSCheck bool `json:"dnscheck"`
}

type ipSourceDTO struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type settingsDTO struct {
	IPCheckIntervalSeconds int            `json:"ip_check_interval_seconds"`
	TunnelIntervalSeconds  int            `json:"tunnel_interval_seconds"`
	IPSources              []ipSourceDTO  `json:"ip_sources"` // Reihenfolge = Priorität
	NotifyLanguage         string         `json:"notify_language"`
	ProbeIntervalSeconds   int            `json:"probe_interval_seconds"`
	TLSWarnDays            int            `json:"tls_warn_days"`
	Limits                 map[string]int `json:"limits"`
}

func (s *Server) settingsToDTO(c config.Settings) settingsDTO {
	all := s.settings.SourceNames()
	eff := c.EffectiveSources(all)
	var sources []ipSourceDTO
	for _, n := range eff {
		sources = append(sources, ipSourceDTO{Name: n, Enabled: true})
	}
	for _, n := range all {
		if !contains(eff, n) {
			sources = append(sources, ipSourceDTO{Name: n, Enabled: false})
		}
	}
	return settingsDTO{
		IPCheckIntervalSeconds: int(c.IPCheckInterval.Seconds()),
		TunnelIntervalSeconds:  int(c.TunnelInterval.Seconds()),
		IPSources:              sources,
		NotifyLanguage:         c.NotifyLanguage,
		ProbeIntervalSeconds:   int(c.ProbeInterval.Seconds()),
		TLSWarnDays:            c.TLSWarnDays,
		Limits: map[string]int{
			"ip_check_interval_min": int(config.MinIPCheckInterval.Seconds()),
			"ip_check_interval_max": int(config.MaxIPCheckInterval.Seconds()),
			"tunnel_interval_min":   int(config.MinTunnelInterval.Seconds()),
			"tunnel_interval_max":   int(config.MaxTunnelInterval.Seconds()),
			"ip_sources_min":        config.MinIPSources,
			"probe_interval_min":    int(config.MinProbeInterval.Seconds()),
			"probe_interval_max":    int(config.MaxProbeInterval.Seconds()),
			"tls_warn_days_min":     config.MinTLSWarnDays,
			"tls_warn_days_max":     config.MaxTLSWarnDays,
		},
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeMsg(w, r, http.StatusServiceUnavailable, i18n.M("api.settings_unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, s.settingsToDTO(s.settings.Get()))
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeMsg(w, r, http.StatusServiceUnavailable, i18n.M("api.settings_unavailable"))
		return
	}
	var in struct {
		IPCheckIntervalSeconds int            `json:"ip_check_interval_seconds"`
		TunnelIntervalSeconds  int            `json:"tunnel_interval_seconds"`
		IPSources              []ipSourceDTO  `json:"ip_sources"`
		NotifyLanguage         string         `json:"notify_language"`
		ProbeIntervalSeconds   int            `json:"probe_interval_seconds"` // 0 = unverändert
		TLSWarnDays            int            `json:"tls_warn_days"`          // 0 = unverändert
		Limits                 map[string]int `json:"limits"`                 // wird ignoriert (Rückgabe von GET)
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	next := s.settings.Get()
	next.IPCheckInterval = time.Duration(in.IPCheckIntervalSeconds) * time.Second
	next.TunnelInterval = time.Duration(in.TunnelIntervalSeconds) * time.Second
	if in.NotifyLanguage != "" {
		next.NotifyLanguage = in.NotifyLanguage
	}
	if in.ProbeIntervalSeconds != 0 {
		next.ProbeInterval = time.Duration(in.ProbeIntervalSeconds) * time.Second
	}
	if in.TLSWarnDays != 0 {
		next.TLSWarnDays = in.TLSWarnDays
	}
	next.IPSources = nil
	for _, src := range in.IPSources {
		if src.Enabled {
			next.IPSources = append(next.IPSources, src.Name)
		}
	}

	saved, err := s.settings.Update(r.Context(), next)
	var verr *config.ValidationError
	if errors.As(err, &verr) {
		writeMsgs(w, r, http.StatusBadRequest, verr.Msgs)
		return
	}
	if err != nil {
		s.internalError(w, r, "Einstellungen speichern", err)
		return
	}
	s.log.Info("Einstellungen geändert", "ip_check_interval", saved.IPCheckInterval,
		"tunnel_interval", saved.TunnelInterval, "ip_sources", saved.IPSources,
		"probe_interval", saved.ProbeInterval, "tls_warn_days", saved.TLSWarnDays)
	if s.events != nil {
		s.events.Publish(events.TopicSettings)
	}
	writeJSON(w, http.StatusOK, s.settingsToDTO(saved))
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.info)
}
