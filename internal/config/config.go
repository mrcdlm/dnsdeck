// Package config lädt die Laufzeitkonfiguration aus Umgebungsvariablen.
// Secrets (CF_API_TOKEN, APP_PASSWORD) werden nur hier gelesen und dürfen
// niemals geloggt oder über die API ausgeliefert werden.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/mrcdlm/dnsdeck/internal/dnsbl"
	"github.com/mrcdlm/dnsdeck/internal/dnscheck"
)

type Config struct {
	Port        int
	LogLevel    slog.Level
	DataDir     string
	AppPassword string
	CFAPIToken  string
	CFAccountID string
	// CFAPIBaseURL ersetzt die Cloudflare-API-Adresse (nur für Entwicklung
	// gegen cmd/cfmock); leer = offizielle API.
	CFAPIBaseURL string
	// DNSCheckResolvers: Resolver für die Verbreitungsprüfung (nil = aus).
	DNSCheckResolvers []dnscheck.Resolver
	// DNSCheckAuthoritative: zusätzlich die Nameserver der Zone fragen.
	DNSCheckAuthoritative bool
	// ISPLookup: Anbieter (AS) der öffentlichen IPs per DNS ermitteln.
	ISPLookup bool
	// DNSBLLists: Sperrlisten für die öffentliche IPv4 (nil = aus).
	DNSBLLists []dnsbl.List
	// TrustedProxies: Adressen/Netze von Reverse Proxys, deren Angaben zur
	// Client-IP (CF-Connecting-IP, X-Forwarded-For) übernommen werden.
	TrustedProxies []netip.Prefix
}

// Load liest die Konfiguration über getenv (in Tests austauschbar).
func Load(getenv func(string) string) (*Config, error) {
	c := &Config{
		Port:        8080,
		LogLevel:    slog.LevelInfo,
		DataDir:     "/data",
		AppPassword: getenv("APP_PASSWORD"),
		CFAPIToken:  getenv("CF_API_TOKEN"),
		CFAccountID: getenv("CF_ACCOUNT_ID"),

		CFAPIBaseURL: getenv("CF_API_BASE_URL"),
	}

	if v := getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid PORT: %q", v)
		}
		c.Port = p
	}
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(strings.ToLower(v))); err != nil {
			return nil, fmt.Errorf("invalid LOG_LEVEL: %q", v)
		}
	}
	if v := getenv("DATA_DIR"); v != "" {
		c.DataDir = v
	}
	resolvers, err := dnscheck.ParseResolvers(getenv("DNSCHECK_RESOLVERS"))
	if err != nil {
		return nil, err
	}
	c.DNSCheckResolvers = resolvers
	switch v := strings.ToLower(strings.TrimSpace(getenv("DNSCHECK_AUTHORITATIVE"))); v {
	case "", "on":
		c.DNSCheckAuthoritative = resolvers != nil
	case "off":
	default:
		return nil, fmt.Errorf("invalid DNSCHECK_AUTHORITATIVE: %q (on|off)", v)
	}
	switch v := strings.ToLower(strings.TrimSpace(getenv("ISP_LOOKUP"))); v {
	case "", "on":
		c.ISPLookup = true
	case "off":
	default:
		return nil, fmt.Errorf("invalid ISP_LOOKUP: %q (on|off)", v)
	}
	if c.TrustedProxies, err = ParseTrustedProxies(getenv("TRUSTED_PROXIES")); err != nil {
		return nil, err
	}
	if c.DNSBLLists, err = dnsbl.ParseLists(getenv("DNSBL_LISTS")); err != nil {
		return nil, err
	}
	if c.AppPassword == "" {
		return nil, errors.New("APP_PASSWORD must be set")
	}
	return c, nil
}

// FromEnv ist Load mit os.Getenv.
func FromEnv() (*Config, error) { return Load(os.Getenv) }

// PortFromEnv liefert nur den Port (für den healthcheck-Unterbefehl,
// der ohne APP_PASSWORD auskommen muss).
func PortFromEnv() int {
	if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil && p > 0 {
		return p
	}
	return 8080
}

// LogValue sorgt dafür, dass beim Loggen der Config keine Secrets erscheinen.
func (c *Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("port", c.Port),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("data_dir", c.DataDir),
		slog.Bool("cf_api_token_set", c.CFAPIToken != ""),
		slog.Bool("cf_account_id_set", c.CFAccountID != ""),
		slog.String("cf_api_base_url", c.CFAPIBaseURL),
		slog.Int("dnscheck_resolvers", len(c.DNSCheckResolvers)),
		slog.Bool("dnscheck_authoritative", c.DNSCheckAuthoritative),
		slog.Bool("isp_lookup", c.ISPLookup),
		slog.Int("dnsbl_lists", len(c.DNSBLLists)),
		slog.Int("trusted_proxies", len(c.TrustedProxies)),
	)
}

// ParseTrustedProxies liest TRUSTED_PROXIES: kommagetrennte IP-Adressen oder
// Netze (CIDR), z. B. "172.16.0.0/12,127.0.0.1". Leer = keinem Proxy vertrauen.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry %q: expected IP address or CIDR", part)
		}
		out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
	}
	return out, nil
}
