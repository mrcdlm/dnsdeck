// Package config lädt die Laufzeitkonfiguration aus Umgebungsvariablen.
// Secrets (CF_API_TOKEN, APP_PASSWORD) werden nur hier gelesen und dürfen
// niemals geloggt oder über die API ausgeliefert werden.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
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
			return nil, fmt.Errorf("PORT ungültig: %q", v)
		}
		c.Port = p
	}
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(strings.ToLower(v))); err != nil {
			return nil, fmt.Errorf("LOG_LEVEL ungültig: %q", v)
		}
	}
	if v := getenv("DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if c.AppPassword == "" {
		return nil, errors.New("APP_PASSWORD muss gesetzt sein")
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
	)
}
