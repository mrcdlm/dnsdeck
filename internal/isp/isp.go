// Package isp ermittelt, zu welchem Netz (Autonomes System) eine öffentliche
// Adresse gehört – also in der Regel den Internetanbieter.
//
// Die Abfrage läuft rein über DNS beim IP-to-ASN-Dienst von Team Cymru
// (origin.asn.cymru.com / origin6.asn.cymru.com / asn.cymru.com), ohne
// API-Schlüssel und ohne zusätzlichen HTTP-Dienst. Ergänzend wird der
// Reverse-DNS-Name der Adresse abgefragt, der oft den Anbieter erkennen lässt.
package isp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// Info beschreibt das Netz einer Adresse. Alle Felder sind sprachneutral.
type Info struct {
	ASN      int    `json:"asn"`
	Name     string `json:"name,omitempty"`     // Name des AS, z. B. "DTAG Internet service provider operations"
	Prefix   string `json:"prefix,omitempty"`   // angekündigtes Netz, z. B. "79.192.0.0/10"
	Country  string `json:"country,omitempty"`  // ISO-3166-Ländercode
	Registry string `json:"registry,omitempty"` // z. B. "ripencc"
	Hostname string `json:"hostname,omitempty"` // Reverse-DNS-Name der Adresse
}

// Resolver ist der Teil von *net.Resolver, der benötigt wird (in Tests austauschbar).
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

// Lookup fragt das Netz einer Adresse ab.
type Lookup struct {
	Resolver Resolver
	Timeout  time.Duration
}

func New() *Lookup {
	return &Lookup{Resolver: net.DefaultResolver, Timeout: 5 * time.Second}
}

// ErrNotFound: Für die Adresse ist kein Netz bekannt.
var ErrNotFound = errors.New("no origin AS found")

// Lookup liefert die Netzinformationen zu a. Ein fehlender Reverse-DNS-Name
// oder AS-Name ist kein Fehler; nur wenn das AS selbst nicht ermittelt werden
// kann, wird ein Fehler gemeldet.
func (l *Lookup) Lookup(ctx context.Context, a netip.Addr) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()

	a = a.Unmap()
	host := make(chan string, 1)
	go func() { host <- l.hostname(ctx, a) }()

	info, err := l.origin(ctx, a)
	if err == nil {
		info.Name = l.asName(ctx, info.ASN)
	}
	info.Hostname = <-host
	return info, err
}

func (l *Lookup) origin(ctx context.Context, a netip.Addr) (Info, error) {
	txts, err := l.Resolver.LookupTXT(ctx, originName(a))
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return Info{}, ErrNotFound
		}
		return Info{}, err
	}
	// Bei mehreren Einträgen gewinnt das spezifischste Netz.
	var best Info
	bestBits := -1
	for _, t := range txts {
		info, bits, ok := parseOrigin(t)
		if ok && bits > bestBits {
			best, bestBits = info, bits
		}
	}
	if bestBits < 0 {
		return Info{}, ErrNotFound
	}
	return best, nil
}

func (l *Lookup) asName(ctx context.Context, asn int) string {
	txts, err := l.Resolver.LookupTXT(ctx, "AS"+strconv.Itoa(asn)+".asn.cymru.com")
	if err != nil {
		return ""
	}
	for _, t := range txts {
		if name := parseASName(t); name != "" {
			return name
		}
	}
	return ""
}

func (l *Lookup) hostname(ctx context.Context, a netip.Addr) string {
	names, err := l.Resolver.LookupAddr(ctx, a.String())
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// originName baut den Abfragenamen: umgekehrte Oktette (IPv4) bzw. Nibbles (IPv6).
func originName(a netip.Addr) string {
	var b strings.Builder
	if a.Is4() {
		o := a.As4()
		for i := 3; i >= 0; i-- {
			b.WriteString(strconv.Itoa(int(o[i])))
			b.WriteByte('.')
		}
		b.WriteString("origin.asn.cymru.com")
		return b.String()
	}
	const hex = "0123456789abcdef"
	o := a.As16()
	for i := 15; i >= 0; i-- {
		b.WriteByte(hex[o[i]&0x0f])
		b.WriteByte('.')
		b.WriteByte(hex[o[i]>>4])
		b.WriteByte('.')
	}
	b.WriteString("origin6.asn.cymru.com")
	return b.String()
}

// parseOrigin liest "3320 | 79.192.0.0/10 | DE | ripencc | 2007-07-03".
// Werden mehrere AS genannt ("3320 6805 | …"), zählt das erste.
func parseOrigin(t string) (Info, int, bool) {
	f := fields(t)
	if len(f) < 2 {
		return Info{}, 0, false
	}
	asns := strings.Fields(f[0])
	if len(asns) == 0 {
		return Info{}, 0, false
	}
	asn, err := strconv.Atoi(asns[0])
	if err != nil || asn <= 0 {
		return Info{}, 0, false
	}
	p, err := netip.ParsePrefix(f[1])
	if err != nil {
		return Info{}, 0, false
	}
	info := Info{ASN: asn, Prefix: p.String()}
	if len(f) > 2 {
		info.Country = strings.ToUpper(f[2])
	}
	if len(f) > 3 {
		info.Registry = f[3]
	}
	return info, p.Bits(), true
}

// parseASName liest "3320 | DE | ripencc | 1993-02-10 | DTAG …, DE" und
// entfernt den angehängten Ländercode.
func parseASName(t string) string {
	f := fields(t)
	if len(f) < 5 {
		return ""
	}
	name := f[4]
	if i := strings.LastIndex(name, ", "); i > 0 && len(name)-i-2 == 2 && strings.EqualFold(name[i+2:], f[1]) {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

func fields(t string) []string {
	parts := strings.Split(t, "|")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// String für Logs.
func (i Info) String() string {
	return fmt.Sprintf("AS%d %s", i.ASN, i.Name)
}
