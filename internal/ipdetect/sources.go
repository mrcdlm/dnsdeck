// Package ipdetect ermittelt die öffentliche IPv4-/IPv6-Adresse über mehrere
// unabhängige Quellen und entscheidet per Mehrheit, damit eine einzelne
// fehlerhafte Quelle keinen DNS-Update auslöst.
package ipdetect

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
)

type Family string

const (
	IPv4 Family = "ipv4"
	IPv6 Family = "ipv6"
)

var Families = []Family{IPv4, IPv6}

// Source ist ein HTTP-Endpunkt, der die anfragende IP zurückgibt.
type Source struct {
	Name   string // Gruppenname, z. B. "cloudflare" (für Settings)
	Family Family
	URL    string
	Parse  func(body []byte) (string, error)
}

// DefaultSources in Standardreihenfolge. Pro Name gibt es je eine Variante für
// IPv4 und IPv6; die Adressfamilie wird zusätzlich über den Dialer erzwungen.
func DefaultSources() []Source {
	return []Source{
		{Name: "cloudflare", Family: IPv4, URL: "https://1.1.1.1/cdn-cgi/trace", Parse: parseTrace},
		{Name: "cloudflare", Family: IPv6, URL: "https://[2606:4700:4700::1111]/cdn-cgi/trace", Parse: parseTrace},
		{Name: "ipify", Family: IPv4, URL: "https://api.ipify.org", Parse: parsePlain},
		{Name: "ipify", Family: IPv6, URL: "https://api6.ipify.org", Parse: parsePlain},
		{Name: "icanhazip", Family: IPv4, URL: "https://icanhazip.com", Parse: parsePlain},
		{Name: "icanhazip", Family: IPv6, URL: "https://icanhazip.com", Parse: parsePlain},
	}
}

// SourceNames liefert die eindeutigen Quellnamen in Standardreihenfolge.
func SourceNames() []string {
	var names []string
	seen := map[string]bool{}
	for _, s := range DefaultSources() {
		if !seen[s.Name] {
			seen[s.Name] = true
			names = append(names, s.Name)
		}
	}
	return names
}

// SelectSources filtert und sortiert srcs nach names. Unbekannte Namen werden
// ignoriert; ist names leer (oder bleibt nichts übrig), gilt srcs unverändert.
func SelectSources(srcs []Source, names []string) []Source {
	var out []Source
	for _, n := range names {
		for _, s := range srcs {
			if s.Name == n {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		return srcs
	}
	return out
}

func parsePlain(body []byte) (string, error) {
	return strings.TrimSpace(string(body)), nil
}

// parseTrace liest die Zeile "ip=…" aus /cdn-cgi/trace.
func parseTrace(body []byte) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "ip="); ok {
			return strings.TrimSpace(v), nil
		}
	}
	return "", errors.New("keine ip=-Zeile in trace-Antwort")
}
