package config

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Obergrenzen für Intervalle (Untergrenzen siehe Min*).
const (
	MaxIPCheckInterval = 24 * time.Hour
	MaxTunnelInterval  = time.Hour
	MinIPSources       = 2 // Mehrheitsentscheid braucht mindestens zwei Quellen
)

type settingsRW interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// ValidationError ist ein Eingabefehler, der dem Benutzer angezeigt wird.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

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
			warnings = append(warnings, fmt.Errorf("unbekannte IP-Quelle %q ignoriert", n))
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
	var msgs []string
	if n.IPCheckInterval < MinIPCheckInterval || n.IPCheckInterval > MaxIPCheckInterval {
		msgs = append(msgs, fmt.Sprintf("IP-Prüfintervall muss zwischen %s und %s liegen", MinIPCheckInterval, MaxIPCheckInterval))
	}
	if n.TunnelInterval < MinTunnelInterval || n.TunnelInterval > MaxTunnelInterval {
		msgs = append(msgs, fmt.Sprintf("Tunnel-Intervall muss zwischen %s und %s liegen", MinTunnelInterval, MaxTunnelInterval))
	}
	seen := map[string]bool{}
	for _, name := range n.IPSources {
		if !slices.Contains(s.sourceNames, name) {
			msgs = append(msgs, fmt.Sprintf("unbekannte IP-Quelle %q", name))
		}
		if seen[name] {
			msgs = append(msgs, fmt.Sprintf("IP-Quelle %q doppelt", name))
		}
		seen[name] = true
	}
	if len(n.IPSources) < MinIPSources {
		msgs = append(msgs, fmt.Sprintf("mindestens %d IP-Quellen auswählen (Mehrheitsentscheid)", MinIPSources))
	}
	if len(msgs) > 0 {
		return &ValidationError{Msg: strings.Join(msgs, "; ")}
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
