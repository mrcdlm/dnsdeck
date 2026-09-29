package config

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

type mapStore map[string]string

func (m mapStore) GetSetting(_ context.Context, k string) (string, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return "", store.ErrNotFound
}

func TestLoadSettings(t *testing.T) {
	s, w := LoadSettings(context.Background(), mapStore{})
	if len(w) != 0 || s.IPCheckInterval != DefaultIPCheckInterval || s.IPSources != nil {
		t.Fatalf("Defaults falsch: %+v %v", s, w)
	}

	s, w = LoadSettings(context.Background(), mapStore{
		KeyIPCheckInterval: "2m",
		KeyIPSources:       " ipify, cloudflare ,,",
	})
	if len(w) != 0 || s.IPCheckInterval != 2*time.Minute ||
		!reflect.DeepEqual(s.IPSources, []string{"ipify", "cloudflare"}) {
		t.Fatalf("unerwartet: %+v %v", s, w)
	}

	s, w = LoadSettings(context.Background(), mapStore{KeyIPCheckInterval: "1s"})
	if len(w) != 1 || s.IPCheckInterval != DefaultIPCheckInterval {
		t.Fatalf("zu kleines Intervall nicht abgefangen: %+v %v", s, w)
	}
}
