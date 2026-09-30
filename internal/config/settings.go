package config

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

// Keys der in der DB gespeicherten Einstellungen.
const (
	KeyIPCheckInterval = "ip_check_interval"
	KeyIPSources       = "ip_sources"
	KeyTunnelInterval  = "tunnel_poll_interval"
	KeyNotifyLanguage  = "notify_language"
)

const (
	DefaultIPCheckInterval = 5 * time.Minute
	MinIPCheckInterval     = 30 * time.Second

	DefaultTunnelInterval = 60 * time.Second
	MinTunnelInterval     = 15 * time.Second
)

// Settings sind zur Laufzeit änderbare Einstellungen (keine Secrets).
type Settings struct {
	IPCheckInterval time.Duration
	// IPSources: Namen der IP-Quellen in gewünschter Reihenfolge;
	// leer = alle bekannten Quellen in Standardreihenfolge.
	IPSources []string
	// TunnelInterval: Abfrageintervall für Cloudflare Tunnels.
	TunnelInterval time.Duration
	// NotifyLanguage: Sprache der Benachrichtigungen ("de" oder "en").
	NotifyLanguage string
}

// DefaultNotifyLanguage ist die Standardsprache der Benachrichtigungen.
const DefaultNotifyLanguage = "de"

type settingsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
}

// LoadSettings liest die Einstellungen aus der DB und füllt Defaults auf.
// Ungültige Werte werden ignoriert (Default), damit die App immer startet.
func LoadSettings(ctx context.Context, s settingsStore) (Settings, []error) {
	out := Settings{IPCheckInterval: DefaultIPCheckInterval, TunnelInterval: DefaultTunnelInterval,
		NotifyLanguage: DefaultNotifyLanguage}
	var warnings []error

	if v, err := s.GetSetting(ctx, KeyIPCheckInterval); err == nil {
		d, perr := time.ParseDuration(v)
		if perr != nil || d < MinIPCheckInterval {
			warnings = append(warnings, fmt.Errorf("%s invalid (%q), using default", KeyIPCheckInterval, v))
		} else {
			out.IPCheckInterval = d
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		warnings = append(warnings, err)
	}

	if v, err := s.GetSetting(ctx, KeyTunnelInterval); err == nil {
		d, perr := time.ParseDuration(v)
		if perr != nil || d < MinTunnelInterval {
			warnings = append(warnings, fmt.Errorf("%s invalid (%q), using default", KeyTunnelInterval, v))
		} else {
			out.TunnelInterval = d
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		warnings = append(warnings, err)
	}

	if v, err := s.GetSetting(ctx, KeyNotifyLanguage); err == nil {
		if v == "de" || v == "en" {
			out.NotifyLanguage = v
		} else {
			warnings = append(warnings, fmt.Errorf("%s invalid (%q), using default", KeyNotifyLanguage, v))
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		warnings = append(warnings, err)
	}

	if v, err := s.GetSetting(ctx, KeyIPSources); err == nil {
		for _, name := range strings.Split(v, ",") {
			if name = strings.TrimSpace(name); name != "" {
				out.IPSources = append(out.IPSources, name)
			}
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		warnings = append(warnings, err)
	}

	return out, warnings
}
