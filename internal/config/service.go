package config

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

// Obergrenzen für Intervalle (Untergrenzen siehe Min*).
const (
	MaxIPCheckInterval   = 24 * time.Hour
	MaxTunnelInterval    = time.Hour
	MaxProbeInterval     = 24 * time.Hour
	MaxSpeedtestInterval = 7 * 24 * time.Hour
	MinIPSources         = 2 // Mehrheitsentscheid braucht mindestens zwei Quellen
)

type settingsRW interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// ValidationError sind Eingabefehler, die dem Benutzer angezeigt werden.
type ValidationError struct{ Msgs []i18n.Msg }

func (e *ValidationError) Error() string { return i18n.Join(i18n.EN, e.Msgs) }

// SettingsService hält die aktuellen Einstellungen, prüft und speichert
// Änderungen und benachrichtigt Abonnenten, damit Änderungen ohne Neustart
// greifen.
type SettingsService struct {
	store       settingsRW
	sourceNames []string // bekannte IP-Quellen

	mu        sync.RWMutex
	cur       Settings
	listeners []func(old, cur Settings)
}

// NewSettingsService lädt die Einstellungen; ungültige gespeicherte Werte
// werden als Warnungen gemeldet und durch Defaults ersetzt.
func NewSettingsService(ctx context.Context, st settingsRW, sourceNames []string) (*SettingsService, []error) {
	cur, warnings := LoadSettings(ctx, st)
	var known []string
	for _, n := range cur.IPSources {
		if slices.Contains(sourceNames, n) {
			known = append(known, n)
		} else {
			warnings = append(warnings, fmt.Errorf("unknown IP source %q ignored", n))
		}
	}
	cur.IPSources = known
	return &SettingsService{store: st, sourceNames: sourceNames, cur: cur}, warnings
}

func (s *SettingsService) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.cur
	c.IPSources = slices.Clone(c.IPSources)
	return c
}

// OnChange registriert fn; es wird nach jeder erfolgreichen Änderung aufgerufen.
func (s *SettingsService) OnChange(fn func(old, cur Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// SourceNames liefert die bekannten IP-Quellen in Standardreihenfolge.
func (s *SettingsService) SourceNames() []string { return slices.Clone(s.sourceNames) }

// Update prüft und speichert die Einstellungen vollständig.
func (s *SettingsService) Update(ctx context.Context, next Settings) (Settings, error) {
	if err := s.validate(next); err != nil {
		return Settings{}, err
	}
	for _, kv := range [][2]string{
		{KeyIPCheckInterval, next.IPCheckInterval.String()},
		{KeyTunnelInterval, next.TunnelInterval.String()},
		{KeyIPSources, strings.Join(next.IPSources, ",")},
		{KeyNotifyLanguage, next.NotifyLanguage},
		{KeyProbeInterval, next.ProbeInterval.String()},
		{KeyTLSWarnDays, strconv.Itoa(next.TLSWarnDays)},
		{KeySpeedtestInterval, next.SpeedtestInterval.String()},
	} {
		if err := s.store.SetSetting(ctx, kv[0], kv[1]); err != nil {
			return Settings{}, err
		}
	}

	s.mu.Lock()
	old := s.cur
	s.cur = next
	listeners := slices.Clone(s.listeners)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(old, next)
	}
	return s.Get(), nil
}

func (s *SettingsService) validate(n Settings) error {
	var msgs []i18n.Msg
	add := func(code string, kv ...string) { msgs = append(msgs, i18n.M(code, kv...)) }
	if n.IPCheckInterval < MinIPCheckInterval || n.IPCheckInterval > MaxIPCheckInterval {
		add("settings.ip_interval_range", "min", MinIPCheckInterval.String(), "max", MaxIPCheckInterval.String())
	}
	if n.TunnelInterval < MinTunnelInterval || n.TunnelInterval > MaxTunnelInterval {
		add("settings.tunnel_interval_range", "min", MinTunnelInterval.String(), "max", MaxTunnelInterval.String())
	}
	if n.ProbeInterval < MinProbeInterval || n.ProbeInterval > MaxProbeInterval {
		add("settings.probe_interval_range", "min", MinProbeInterval.String(), "max", MaxProbeInterval.String())
	}
	if n.TLSWarnDays < MinTLSWarnDays || n.TLSWarnDays > MaxTLSWarnDays {
		add("settings.tls_warn_days_range", "min", strconv.Itoa(MinTLSWarnDays), "max", strconv.Itoa(MaxTLSWarnDays))
	}
	if d := n.SpeedtestInterval; d != 0 && (d < MinSpeedtestInterval || d > MaxSpeedtestInterval) {
		add("settings.speedtest_interval_range", "min", MinSpeedtestInterval.String(), "max", MaxSpeedtestInterval.String())
	}
	seen := map[string]bool{}
	for _, name := range n.IPSources {
		if !slices.Contains(s.sourceNames, name) {
			add("settings.source_unknown", "name", strconv.Quote(name))
		}
		if seen[name] {
			add("settings.source_duplicate", "name", strconv.Quote(name))
		}
		seen[name] = true
	}
	if len(n.IPSources) < MinIPSources {
		add("settings.sources_min", "min", strconv.Itoa(MinIPSources))
	}
	if n.NotifyLanguage != "de" && n.NotifyLanguage != "en" {
		add("settings.language_invalid")
	}
	if len(msgs) > 0 {
		return &ValidationError{Msgs: msgs}
	}
	return nil
}

// EffectiveSources liefert die IP-Quellen in Reihenfolge; leer = alle.
func (s Settings) EffectiveSources(all []string) []string {
	if len(s.IPSources) == 0 {
		return all
	}
	return s.IPSources
}

// IsValidation meldet, ob err ein Eingabefehler ist.
func IsValidation(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}
