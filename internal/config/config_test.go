package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"APP_PASSWORD": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 8080 || c.DataDir != "/data" || c.LogLevel != slog.LevelInfo {
		t.Fatalf("unerwartete Defaults: %+v", c)
	}
}

func TestLoadRequiresPassword(t *testing.T) {
	if _, err := Load(env(nil)); err == nil {
		t.Fatal("Fehler erwartet ohne APP_PASSWORD")
	}
}

func TestLoadInvalid(t *testing.T) {
	for _, m := range []map[string]string{
		{"APP_PASSWORD": "x", "PORT": "abc"},
		{"APP_PASSWORD": "x", "PORT": "70000"},
		{"APP_PASSWORD": "x", "LOG_LEVEL": "laut"},
	} {
		if _, err := Load(env(m)); err == nil {
			t.Errorf("Fehler erwartet für %v", m)
		}
	}
}

func TestLogValueHidesSecrets(t *testing.T) {
	c, _ := Load(env(map[string]string{
		"APP_PASSWORD": "geheim-pw", "CF_API_TOKEN": "geheim-token", "LOG_LEVEL": "DEBUG",
	}))
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("cfg", "config", c)
	out := buf.String()
	if strings.Contains(out, "geheim") {
		t.Fatalf("Secret im Log: %s", out)
	}
	if !strings.Contains(out, "cf_api_token_set=true") {
		t.Fatalf("Flag fehlt: %s", out)
	}
}
