// Package dnsbl prüft, ob eine öffentliche IPv4-Adresse auf DNS-basierten
// Sperrlisten (DNSBL) steht, die Mailserver vor der Annahme von E-Mails
// abfragen.
//
// Abgefragt wird <umgekehrte Oktette>.<zone> per DNS: keine Antwort heißt
// „nicht gelistet“, eine Adresse 127.0.0.x heißt „gelistet“ (der Code nennt
// den Grund). Antworten 127.255.255.x bedeuten bei Spamhaus, dass die Abfrage
// abgelehnt wurde – typisch über öffentliche Resolver wie 1.1.1.1 oder 8.8.8.8.
package dnsbl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// List ist eine Sperrliste.
type List struct {
	Name string `json:"name"`
	Zone string `json:"zone"`
}

// DefaultLists sind frei abfragbar (keine Registrierung nötig).
var DefaultLists = []List{
	{Name: "Spamhaus ZEN", Zone: "zen.spamhaus.org"},
	{Name: "SpamCop", Zone: "bl.spamcop.net"},
	{Name: "PSBL", Zone: "psbl.surriel.com"},
	{Name: "Mailspike", Zone: "bl.mailspike.net"},
}

// ParseLists liest DNSBL_LISTS: leer = DefaultLists, "off" = abgeschaltet (nil),
// sonst kommagetrennt "[Name=]zone".
func ParseLists(s string) ([]List, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "":
		return DefaultLists, nil
	case "off":
		return nil, nil
	}
	var out []List
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, zone, named := strings.Cut(part, "=")
		if !named {
			zone = name
		}
		zone = strings.ToLower(strings.Trim(strings.TrimSpace(zone), "."))
		if !validZone(zone) {
			return nil, fmt.Errorf("invalid DNSBL_LISTS entry %q: expected a DNS zone such as zen.spamhaus.org", part)
		}
		if !named {
			name = zone
		}
		out = append(out, List{Name: strings.TrimSpace(name), Zone: zone})
	}
	if len(out) == 0 {
		return nil, errors.New("DNSBL_LISTS contains no list")
	}
	return out, nil
}

func validZone(z string) bool {
	if len(z) > 200 || !strings.Contains(z, ".") {
		return false
	}
	for _, label := range strings.Split(z, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// Ergebnis je Liste bzw. gesamt.
const (
	StatusClean   = "clean"   // nicht gelistet
	StatusListed  = "listed"  // gelistet
	StatusPolicy  = "policy"  // nur Spamhaus PBL: Endkundenanschluss – normal, kein Alarm
	StatusRefused = "refused" // Liste hat die Abfrage abgelehnt (öffentlicher Resolver, Limit)
	StatusError   = "error"   // Abfrage fehlgeschlagen
	StatusUnknown = "unknown" // gesamt: keine Liste konnte geprüft werden
)

// Entry ist das Ergebnis einer Liste.
type Entry struct {
	Name   string   `json:"name"`
	Zone   string   `json:"zone"`
	Status string   `json:"status"`
	Codes  []string `json:"codes,omitempty"`  // Antwortadressen, z. B. 127.0.0.2
	Detail string   `json:"detail,omitempty"` // bekannte Teillisten, z. B. "SBL, XBL"
}

// Result ist das Ergebnis einer Prüfung.
type Result struct {
	CheckedAt time.Time `json:"checked_at"`
	Status    string    `json:"status"` // listed | clean | unknown
	Lists     []Entry   `json:"lists"`
}

// Listed liefert die Listen, auf denen die Adresse tatsächlich steht.
func (r Result) Listed() []Entry {
	var out []Entry
	for _, e := range r.Lists {
		if e.Status == StatusListed {
			out = append(out, e)
		}
	}
	return out
}

// Resolver ist der Teil von *net.Resolver, der benötigt wird (in Tests austauschbar).
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Checker fragt die konfigurierten Listen ab.
type Checker struct {
	Resolver Resolver
	Lists    []List
	Timeout  time.Duration
	now      func() time.Time
}

func New(lists []List) *Checker {
	return &Checker{Resolver: net.DefaultResolver, Lists: lists, Timeout: 5 * time.Second, now: time.Now}
}

// Check fragt alle Listen parallel ab. Nur IPv4 wird unterstützt; für andere
// Adressen ist das Ergebnis "unknown" ohne Listen.
func (c *Checker) Check(ctx context.Context, a netip.Addr) Result {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	res := Result{CheckedAt: now().UTC(), Status: StatusUnknown, Lists: []Entry{}}
	a = a.Unmap()
	if !a.Is4() {
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	res.Lists = make([]Entry, len(c.Lists))
	var wg sync.WaitGroup
	for i, l := range c.Lists {
		wg.Go(func() { res.Lists[i] = c.query(ctx, a, l) })
	}
	wg.Wait()
	res.Status = overall(res.Lists)
	return res
}

func overall(entries []Entry) string {
	status := StatusUnknown
	for _, e := range entries {
		switch e.Status {
		case StatusListed:
			return StatusListed
		case StatusClean, StatusPolicy:
			status = StatusClean
		}
	}
	return status
}

func (c *Checker) query(ctx context.Context, a netip.Addr, l List) Entry {
	e := Entry{Name: l.Name, Zone: l.Zone}
	addrs, err := c.Resolver.LookupHost(ctx, queryName(a, l.Zone))
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			e.Status = StatusClean
		} else {
			e.Status = StatusError
		}
		return e
	}
	var codes []netip.Addr
	for _, s := range addrs {
		if ip, err := netip.ParseAddr(s); err == nil && ip.Is4() {
			codes = append(codes, ip)
		}
	}
	slices.SortFunc(codes, func(x, y netip.Addr) int { return x.Compare(y) })
	for _, ip := range codes {
		e.Codes = append(e.Codes, ip.String())
	}
	e.Status, e.Detail = classify(l.Zone, codes)
	return e
}

// classify wertet die Antwortadressen aus.
func classify(zone string, codes []netip.Addr) (status, detail string) {
	var listed, refused bool
	for _, ip := range codes {
		o := ip.As4()
		switch {
		case o[0] == 127 && o[1] == 255 && o[2] == 255:
			refused = true
		case o[0] == 127:
			listed = true
		}
	}
	switch {
	case listed:
	case refused:
		return StatusRefused, ""
	default:
		// Antworten außerhalb von 127/8 sind keine gültigen DNSBL-Antworten
		// (z. B. Umleitung durch einen Resolver mit Werbeseite).
		return StatusError, ""
	}

	if !isSpamhaus(zone) {
		return StatusListed, ""
	}
	var parts []string
	policyOnly := true
	for _, ip := range codes {
		o := ip.As4()
		if o[0] != 127 || o[1] != 0 || o[2] != 0 {
			continue
		}
		part := spamhausPart(o[3])
		if part != "PBL" {
			policyOnly = false
		}
		if part != "" && !slices.Contains(parts, part) {
			parts = append(parts, part)
		}
	}
	if policyOnly {
		return StatusPolicy, "PBL"
	}
	return StatusListed, strings.Join(parts, ", ")
}

func isSpamhaus(zone string) bool {
	return strings.HasSuffix(zone, ".spamhaus.org") || strings.HasSuffix(zone, ".spamhaus.net")
}

// spamhausPart ordnet Rückgabecodes von Spamhaus ZEN einer Teilliste zu.
func spamhausPart(last byte) string {
	switch {
	case last == 2 || last == 3 || last == 9:
		return "SBL"
	case last >= 4 && last <= 7:
		return "XBL"
	case last == 10 || last == 11:
		return "PBL"
	}
	return "127.0.0." + strconv.Itoa(int(last))
}

// queryName baut <d>.<c>.<b>.<a>.<zone>.
func queryName(a netip.Addr, zone string) string {
	o := a.As4()
	return fmt.Sprintf("%d.%d.%d.%d.%s", o[3], o[2], o[1], o[0], zone)
}
