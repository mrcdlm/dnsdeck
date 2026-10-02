package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/mrcdlm/dnsdeck/internal/dnsbl"
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

func TestLoadDNSCheck(t *testing.T) {
	c, err := Load(env(map[string]string{"APP_PASSWORD": "x"}))
	if err != nil || len(c.DNSCheckResolvers) != 4 || !c.DNSCheckAuthoritative {
		t.Fatalf("default: %+v %v", c, err)
	}
	c, err = Load(env(map[string]string{"APP_PASSWORD": "x", "DNSCHECK_RESOLVERS": "off"}))
	if err != nil || c.DNSCheckResolvers != nil || c.DNSCheckAuthoritative {
		t.Fatalf("off: %+v %v", c, err)
	}
	c, err = Load(env(map[string]string{"APP_PASSWORD": "x", "DNSCHECK_RESOLVERS": "127.0.0.1:8553",
		"DNSCHECK_AUTHORITATIVE": "off"}))
	if err != nil || len(c.DNSCheckResolvers) != 1 || c.DNSCheckAuthoritative {
		t.Fatalf("custom: %+v %v", c, err)
	}
	for _, m := range []map[string]string{
		{"APP_PASSWORD": "x", "DNSCHECK_RESOLVERS": "dns.google"},
		{"APP_PASSWORD": "x", "DNSCHECK_AUTHORITATIVE": "vielleicht"},
	} {
		if _, err := Load(env(m)); err == nil {
			t.Errorf("%v: expected error", m)
		}
	}
}

func TestLoadDNSBLLists(t *testing.T) {
	c, err := Load(env(map[string]string{"APP_PASSWORD": "x"}))
	if err != nil || len(c.DNSBLLists) != len(dnsbl.DefaultLists) {
		t.Fatalf("Standard: %v %v", c, err)
	}
	if c, err = Load(env(map[string]string{"APP_PASSWORD": "x", "DNSBL_LISTS": "off"})); err != nil || c.DNSBLLists != nil {
		t.Fatalf("off: %v %v", c, err)
	}
	if _, err := Load(env(map[string]string{"APP_PASSWORD": "x", "DNSBL_LISTS": "kein zone"})); err == nil {
		t.Error("expected error")
	}
}

func TestLoadISPLookup(t *testing.T) {
	for v, want := range map[string]bool{"": true, "on": true, "OFF": false} {
		c, err := Load(env(map[string]string{"APP_PASSWORD": "x", "ISP_LOOKUP": v}))
		if err != nil || c.ISPLookup != want {
			t.Errorf("ISP_LOOKUP=%q: %v %v", v, c, err)
		}
	}
	if _, err := Load(env(map[string]string{"APP_PASSWORD": "x", "ISP_LOOKUP": "ja"})); err == nil {
		t.Error("expected error")
	}
}
