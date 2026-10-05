package config

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

// Keys der in der DB gespeicherten Einstellungen.
const (
	KeyIPCheckInterval   = "ip_check_interval"
	KeyIPSources         = "ip_sources"
	KeyTunnelInterval    = "tunnel_poll_interval"
	KeyNotifyLanguage    = "notify_language"
	KeyProbeInterval     = "probe_interval"
	KeyTLSWarnDays       = "tls_warn_days"
	KeySpeedtestInterval = "speedtest_interval"
)

const (
	DefaultIPCheckInterval = 5 * time.Minute
	MinIPCheckInterval     = 30 * time.Second

	DefaultTunnelInterval = 60 * time.Second
	MinTunnelInterval     = 15 * time.Second

	DefaultProbeInterval = 5 * time.Minute
	MinProbeInterval     = 30 * time.Second

	DefaultTLSWarnDays = 14
	MinTLSWarnDays     = 1
	MaxTLSWarnDays     = 90

	// Speedtest: 0 = nur manuell (Default); jede Messung überträgt je nach
	// Anschluss einige hundert MB, daher höchstens stündlich.
	MinSpeedtestInterval = time.Hour
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
	// ProbeInterval: Abstand der Erreichbarkeitsprüfungen.
	ProbeInterval time.Duration
	// TLSWarnDays: ab dieser Restlaufzeit (Tage) wird vor Zertifikaten gewarnt.
	TLSWarnDays int
	// SpeedtestInterval: Abstand geplanter Speedtests; 0 = nur manuell.
	SpeedtestInterval time.Duration
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
		NotifyLanguage: DefaultNotifyLanguage, ProbeInterval: DefaultProbeInterval, TLSWarnDays: DefaultTLSWarnDays}
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

	if v, err := s.GetSetting(ctx, KeyProbeInterval); err == nil {
		d, perr := time.ParseDuration(v)
		if perr != nil || d < MinProbeInterval {
			warnings = append(warnings, fmt.Errorf("%s invalid (%q), using default", KeyProbeInterval, v))
		} else {
			out.ProbeInterval = d
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		warnings = append(warnings, err)
	}

	if v, err := s.GetSetting(ctx, KeyTLSWarnDays); err == nil {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < MinTLSWarnDays || n > MaxTLSWarnDays {
			warnings = append(warnings, fmt.Errorf("%s invalid (%q), using default", KeyTLSWarnDays, v))
		} else {
			out.TLSWarnDays = n
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		warnings = append(warnings, err)
	}

	if v, err := s.GetSetting(ctx, KeySpeedtestInterval); err == nil {
		d, perr := time.ParseDuration(v)
		if perr != nil || (d != 0 && (d < MinSpeedtestInterval || d > MaxSpeedtestInterval)) {
			warnings = append(warnings, fmt.Errorf("%s invalid (%q), using default", KeySpeedtestInterval, v))
		} else {
			out.SpeedtestInterval = d
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
