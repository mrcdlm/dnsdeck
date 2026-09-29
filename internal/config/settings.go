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
)

const (
	DefaultIPCheckInterval = 5 * time.Minute
	MinIPCheckInterval     = 30 * time.Second
)

// Settings sind zur Laufzeit änderbare Einstellungen (keine Secrets).
type Settings struct {
	IPCheckInterval time.Duration
	// IPSources: Namen der IP-Quellen in gewünschter Reihenfolge;
	// leer = alle bekannten Quellen in Standardreihenfolge.
	IPSources []string
}

type settingsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
}

// LoadSettings liest die Einstellungen aus der DB und füllt Defaults auf.
// Ungültige Werte werden ignoriert (Default), damit die App immer startet.
func LoadSettings(ctx context.Context, s settingsStore) (Settings, []error) {
	out := Settings{IPCheckInterval: DefaultIPCheckInterval}
	var warnings []error

	if v, err := s.GetSetting(ctx, KeyIPCheckInterval); err == nil {
		d, perr := time.ParseDuration(v)
		if perr != nil || d < MinIPCheckInterval {
			warnings = append(warnings, fmt.Errorf("%s ungültig (%q), nutze Default", KeyIPCheckInterval, v))
		} else {
			out.IPCheckInterval = d
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
