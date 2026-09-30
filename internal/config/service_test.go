package config

import (
	"context"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

type memSettings map[string]string

func (m memSettings) GetSetting(_ context.Context, k string) (string, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return "", store.ErrNotFound
}

func (m memSettings) SetSetting(_ context.Context, k, v string) error { m[k] = v; return nil }

var names = []string{"cloudflare", "ipify", "icanhazip"}

func TestSettingsService(t *testing.T) {
	ctx := context.Background()
	st := memSettings{KeyIPSources: "ipify,gibtsnicht,cloudflare"}
	svc, warnings := NewSettingsService(ctx, st, names)
	if len(warnings) != 1 || len(svc.Get().IPSources) != 2 {
		t.Fatalf("warnings=%v sources=%v", warnings, svc.Get().IPSources)
	}

	var called Settings
	svc.OnChange(func(_, cur Settings) { called = cur })

	next := svc.Get()
	next.IPCheckInterval = 2 * time.Minute
	next.TunnelInterval = 30 * time.Second
	next.IPSources = []string{"icanhazip", "cloudflare"}
	got, err := svc.Update(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	if called.IPCheckInterval != 2*time.Minute || got.TunnelInterval != 30*time.Second {
		t.Fatalf("got=%+v called=%+v", got, called)
	}
	if st[KeyIPCheckInterval] != "2m0s" || st[KeyIPSources] != "icanhazip,cloudflare" {
		t.Fatalf("gespeichert: %v", st)
	}

	// neu laden liefert denselben Stand
	svc2, w := NewSettingsService(ctx, st, names)
	if len(w) != 0 || svc2.Get().TunnelInterval != 30*time.Second {
		t.Fatalf("neu geladen: %+v %v", svc2.Get(), w)
	}

	for _, bad := range []func(*Settings){
		func(s *Settings) { s.IPCheckInterval = time.Second },
		func(s *Settings) { s.TunnelInterval = 2 * time.Hour },
		func(s *Settings) { s.IPSources = []string{"ipify"} },
		func(s *Settings) { s.IPSources = []string{"ipify", "ipify"} },
		func(s *Settings) { s.IPSources = []string{"ipify", "nope"} },
	} {
		n := svc.Get()
		bad(&n)
		if _, err := svc.Update(ctx, n); !IsValidation(err) {
			t.Errorf("Validierungsfehler erwartet: %+v → %v", n, err)
		}
	}
}
