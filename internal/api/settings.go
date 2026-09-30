package api

import (
	"context"
	"net/http"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/config"
	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/notify"
)

type settingsService interface {
	Get() config.Settings
	Update(ctx context.Context, s config.Settings) (config.Settings, error)
	SourceNames() []string
}

type notifyService interface {
	Status() []notify.ChannelStatus
	SendTest(ctx context.Context) []notify.TestResult
}

// Info sind nicht geheime Angaben zur Installation.
type Info struct {
	Version       string `json:"version"`
	CFTokenSet    bool   `json:"cf_token_set"`
	CFAccountSet  bool   `json:"cf_account_set"`
	DataDir       string `json:"data_dir"`
	NotifyChannel int    `json:"notify_channels"`
}

type ipSourceDTO struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type settingsDTO struct {
	IPCheckIntervalSeconds int             `json:"ip_check_interval_seconds"`
	TunnelIntervalSeconds  int             `json:"tunnel_interval_seconds"`
	IPSources              []ipSourceDTO   `json:"ip_sources"` // Reihenfolge = Priorität
	NotifyEvents           map[string]bool `json:"notify_events"`
	Limits                 map[string]int  `json:"limits"`
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
	ev := map[string]bool{}
	for _, t := range notify.EventTypes {
		ev[t] = c.NotifyEnabled(t)
	}
	return settingsDTO{
		IPCheckIntervalSeconds: int(c.IPCheckInterval.Seconds()),
		TunnelIntervalSeconds:  int(c.TunnelInterval.Seconds()),
		IPSources:              sources,
		NotifyEvents:           ev,
		Limits: map[string]int{
			"ip_check_interval_min": int(config.MinIPCheckInterval.Seconds()),
			"ip_check_interval_max": int(config.MaxIPCheckInterval.Seconds()),
			"tunnel_interval_min":   int(config.MinTunnelInterval.Seconds()),
			"tunnel_interval_max":   int(config.MaxTunnelInterval.Seconds()),
			"ip_sources_min":        config.MinIPSources,
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

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "Einstellungen nicht verfügbar")
		return
	}
	writeJSON(w, http.StatusOK, s.settingsToDTO(s.settings.Get()))
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "Einstellungen nicht verfügbar")
		return
	}
	var in struct {
		IPCheckIntervalSeconds int             `json:"ip_check_interval_seconds"`
		TunnelIntervalSeconds  int             `json:"tunnel_interval_seconds"`
		IPSources              []ipSourceDTO   `json:"ip_sources"`
		NotifyEvents           map[string]bool `json:"notify_events"`
		Limits                 map[string]int  `json:"limits"` // wird ignoriert (Rückgabe von GET)
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	next := s.settings.Get()
	next.IPCheckInterval = time.Duration(in.IPCheckIntervalSeconds) * time.Second
	next.TunnelInterval = time.Duration(in.TunnelIntervalSeconds) * time.Second
	next.IPSources = nil
	for _, src := range in.IPSources {
		if src.Enabled {
			next.IPSources = append(next.IPSources, src.Name)
		}
	}
	next.NotifyEvents = map[string]bool{}
	for _, t := range notify.EventTypes {
		if on, ok := in.NotifyEvents[t]; ok {
			next.NotifyEvents[t] = on
		}
	}

	saved, err := s.settings.Update(r.Context(), next)
	if config.IsValidation(err) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.internalError(w, "Einstellungen speichern", err)
		return
	}
	s.log.Info("Einstellungen geändert", "ip_check_interval", saved.IPCheckInterval,
		"tunnel_interval", saved.TunnelInterval, "ip_sources", saved.IPSources)
	if s.events != nil {
		s.events.Publish(events.TopicSettings)
	}
	writeJSON(w, http.StatusOK, s.settingsToDTO(saved))
}

type notificationsDTO struct {
	Channels    []notify.ChannelStatus `json:"channels"`
	ConfigError string                 `json:"config_error,omitempty"`
}

func (s *Server) handleNotifications(w http.ResponseWriter, _ *http.Request) {
	o := notificationsDTO{Channels: []notify.ChannelStatus{}, ConfigError: s.notifyConfigError}
	if s.notify != nil {
		o.Channels = s.notify.Status()
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleNotificationTest(w http.ResponseWriter, r *http.Request) {
	if s.notify == nil || len(s.notify.Status()) == 0 {
		writeError(w, http.StatusServiceUnavailable, "keine Benachrichtigungskanäle konfiguriert (NOTIFY_*)")
		return
	}
	ctx, cancel := detached(r)
	defer cancel()
	writeJSON(w, http.StatusOK, s.notify.SendTest(ctx))
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.info)
}
