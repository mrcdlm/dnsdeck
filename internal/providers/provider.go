// Package providers definiert die Schnittstelle für DNS-Anbieter.
package providers

import (
	"context"
	"errors"
)

// ErrNotFound: der Eintrag existiert beim Anbieter nicht.
var ErrNotFound = errors.New("Eintrag beim Anbieter nicht gefunden")

// ErrNotConfigured: dem Anbieter fehlen Zugangsdaten.
var ErrNotConfigured = errors.New("Anbieter nicht konfiguriert")

// Record ist ein DNS-Eintrag (A oder AAAA) bei einem Anbieter.
type Record struct {
	ID      string // leer = Eintrag existiert noch nicht
	Zone    string // Zonen-ID beim Anbieter
	Name    string // FQDN, z. B. "home.example.com"
	Type    string // "A" oder "AAAA"
	Content string // IP-Adresse
	TTL     int    // 1 = automatisch
	Proxied bool
}

// Provider ist die Schnittstelle, die jeder DNS-Anbieter implementiert.
type Provider interface {
	// Name des Anbieters, z. B. "cloudflare".
	Name() string
	// GetRecord liefert den aktuellen Ist-Zustand eines Eintrags oder ErrNotFound.
	GetRecord(ctx context.Context, zone, name, recordType string) (Record, error)
	// UpdateRecord setzt Inhalt, TTL und Proxied-Flag eines Eintrags.
	// Ist r.ID leer, wird der Eintrag angelegt.
	UpdateRecord(ctx context.Context, r Record) (Record, error)
}

// Zone ist eine DNS-Zone beim Anbieter.
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ZoneLister ist optional: Anbieter, die ihre Zonen auflisten können.
type ZoneLister interface {
	ListZones(ctx context.Context) ([]Zone, error)
}
