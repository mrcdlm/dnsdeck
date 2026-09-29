// Package providers definiert die Schnittstelle für DNS-Anbieter.
// Die Cloudflare-Implementierung folgt in Meilenstein 2 (providers/cloudflare).
package providers

import "context"

// Record ist ein DNS-Eintrag (A oder AAAA) bei einem Anbieter.
type Record struct {
	ID      string
	Zone    string
	Name    string // FQDN, z. B. "home.example.com"
	Type    string // "A" oder "AAAA"
	Content string // IP-Adresse
	TTL     int    // 1 = automatisch (Cloudflare)
	Proxied bool
}

// Provider ist die Schnittstelle, die jeder DNS-Anbieter implementiert.
type Provider interface {
	// Name des Anbieters, z. B. "cloudflare".
	Name() string
	// GetRecord liefert den aktuellen Ist-Zustand eines Eintrags.
	GetRecord(ctx context.Context, zone, name, recordType string) (Record, error)
	// UpdateRecord setzt Inhalt, TTL und Proxied-Flag eines Eintrags.
	UpdateRecord(ctx context.Context, r Record) (Record, error)
}
