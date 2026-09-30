// Package dnscheck prüft, ob ein DNS-Eintrag schon überall die neue Adresse
// liefert: öffentliche Resolver (mit Cache) und die autoritativen
// Nameserver der Zone (ohne Cache, zeigen den Stand bei Cloudflare).
package dnscheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Resolver ist ein DNS-Server, der befragt wird.
type Resolver struct {
	Name string
	Addr string // host:port, host ist eine IP
}

// DefaultResolvers sind große öffentliche Resolver.
var DefaultResolvers = []Resolver{
	{Name: "Cloudflare", Addr: "1.1.1.1:53"},
	{Name: "Google", Addr: "8.8.8.8:53"},
	{Name: "Quad9", Addr: "9.9.9.9:53"},
	{Name: "OpenDNS", Addr: "208.67.222.222:53"},
}

// ParseResolvers liest DNSCHECK_RESOLVERS: leer = Standardliste, "off" =
// Prüfung abgeschaltet (nil), sonst kommagetrennt [Name=]IP[:Port].
func ParseResolvers(s string) ([]Resolver, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "":
		return DefaultResolvers, nil
	case "off":
		return nil, nil
	}
	var out []Resolver
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, addr, named := strings.Cut(part, "=")
		if !named {
			addr = name
		}
		ap, err := parseAddr(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid DNSCHECK_RESOLVERS entry %q: %w", part, err)
		}
		if !named {
			name = ap.Addr().String()
		}
		out = append(out, Resolver{Name: strings.TrimSpace(name), Addr: ap.String()})
	}
	if len(out) == 0 {
		return nil, errors.New("DNSCHECK_RESOLVERS contains no resolver")
	}
	return out, nil
}

func parseAddr(s string) (netip.AddrPort, error) {
	s = strings.TrimSpace(s)
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap, nil
	}
	a, err := netip.ParseAddr(strings.Trim(s, "[]"))
	if err != nil {
		return netip.AddrPort{}, errors.New("expected IP or IP:port")
	}
	return netip.AddrPortFrom(a, 53), nil
}

// Gesamtstatus einer Prüfung.
const (
	StatusPropagated = "propagated" // alle antwortenden Server liefern den erwarteten Stand
	StatusPartial    = "partial"    // ein Teil liefert ihn schon
	StatusPending    = "pending"    // noch keiner
	StatusError      = "error"      // kein Server hat geantwortet
)

// Fehlerarten einer einzelnen Abfrage (werden in der Oberfläche übersetzt).
const (
	ErrTimeout = "timeout"
	ErrFailed  = "failed"
)

// Answer ist das Ergebnis eines einzelnen Servers.
type Answer struct {
	Resolver      string   `json:"resolver"`
	Addr          string   `json:"addr,omitempty"`
	Authoritative bool     `json:"authoritative,omitempty"`
	Values        []string `json:"values,omitempty"`
	// TTL: verbleibende Cache-Zeit der Antwort in Sekunden (bei "nicht
	// vorhanden" die negative Cache-Zeit aus dem SOA).
	TTL   int    `json:"ttl"`
	Rcode string `json:"rcode,omitempty"`
	Error string `json:"error,omitempty"`
	Match bool   `json:"match"`
}

// Result ist das Ergebnis einer Prüfung. Es wird als JSON am Record
// gespeichert und enthält nur sprachneutrale Werte.
type Result struct {
	CheckedAt time.Time `json:"checked_at"`
	Type      string    `json:"type"`
	Expected  string    `json:"expected,omitempty"` // erwartete IP; leer bei proxied
	Proxied   bool      `json:"proxied"`
	Status    string    `json:"status"`
	Matching  int       `json:"matching"`
	Total     int       `json:"total"` // Server mit Antwort
	Errors    int       `json:"errors"`
	// MaxWait: spätestens nach so vielen Sekunden ab CheckedAt sollten alle
	// abweichenden Resolver ihren Cache erneuert haben.
	MaxWait int      `json:"max_wait,omitempty"`
	Answers []Answer `json:"answers"`
}

// Query beschreibt, was geprüft wird.
type Query struct {
	Name     string // FQDN
	Type     string // A oder AAAA
	Zone     string // Zonenname (für die autoritativen Nameserver)
	Expected string // erwartete IP (bei Proxied ignoriert)
	Proxied  bool   // Cloudflare liefert eigene IPs → nur Auflösbarkeit prüfen
}

type exchangeFunc func(ctx context.Context, m *dns.Msg, addr string) (*dns.Msg, error)

// Checker fragt die konfigurierten Server parallel ab.
type Checker struct {
	resolvers     []Resolver
	authoritative bool
	timeout       time.Duration
	exchange      exchangeFunc
	now           func() time.Time
	nsPort        string // Port der autoritativen Server (Tests)

	mu      sync.Mutex
	nsCache map[string]nsEntry
}

type nsEntry struct {
	servers []Resolver
	until   time.Time
}

// New erstellt einen Checker. authoritative: zusätzlich die Nameserver der
// Zone direkt fragen.
func New(resolvers []Resolver, authoritative bool) *Checker {
	c := &Checker{resolvers: resolvers, authoritative: authoritative, timeout: 3 * time.Second,
		now: time.Now, nsPort: "53", nsCache: map[string]nsEntry{}}
	c.exchange = c.udpThenTCP
	return c
}

func (c *Checker) udpThenTCP(ctx context.Context, m *dns.Msg, addr string) (*dns.Msg, error) {
	cl := &dns.Client{Net: "udp", Timeout: c.timeout}
	resp, _, err := cl.ExchangeContext(ctx, m, addr)
	if err == nil && resp.Truncated {
		cl.Net = "tcp"
		resp, _, err = cl.ExchangeContext(ctx, m, addr)
	}
	return resp, err
}

// Check fragt alle Server und bewertet die Antworten.
func (c *Checker) Check(ctx context.Context, q Query) Result {
	qtype := dns.TypeA
	if q.Type == "AAAA" {
		qtype = dns.TypeAAAA
	}
	expected := canonicalIP(q.Expected)

	type target struct {
		Resolver
		auth bool
	}
	targets := make([]target, 0, len(c.resolvers)+2)
	for _, r := range c.resolvers {
		targets = append(targets, target{Resolver: r})
	}
	var nsErr *Answer
	if c.authoritative && q.Zone != "" {
		servers, err := c.nameservers(ctx, q.Zone)
		if err != nil {
			nsErr = &Answer{Resolver: "NS " + q.Zone, Authoritative: true, Error: errKind(err)}
		}
		for _, s := range servers {
			targets = append(targets, target{Resolver: s, auth: true})
		}
	}

	answers := make([]Answer, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() {
			answers[i] = c.ask(ctx, t.Resolver, t.auth, q.Name, qtype, expected, q.Proxied)
		})
	}
	wg.Wait()
	if nsErr != nil {
		answers = append(answers, *nsErr)
	}

	res := Result{CheckedAt: c.now().UTC(), Type: q.Type, Proxied: q.Proxied, Answers: answers}
	if !q.Proxied {
		res.Expected = expected
	}
	evaluate(&res)
	return res
}

func evaluate(res *Result) {
	for _, a := range res.Answers {
		switch {
		case a.Error != "":
			res.Errors++
			continue
		case a.Match:
			res.Matching++
		default:
			res.MaxWait = max(res.MaxWait, a.TTL)
		}
		res.Total++
	}
	switch {
	case res.Total == 0:
		res.Status = StatusError
	case res.Matching == res.Total:
		res.Status = StatusPropagated
	case res.Matching > 0:
		res.Status = StatusPartial
	default:
		res.Status = StatusPending
	}
	if res.Status == StatusPropagated {
		res.MaxWait = 0
	}
}

func (c *Checker) ask(ctx context.Context, r Resolver, auth bool, name string, qtype uint16, expected string, proxied bool) Answer {
	a := Answer{Resolver: r.Name, Addr: r.Addr, Authoritative: auth}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.RecursionDesired = !auth
	resp, err := c.exchange(ctx, m, r.Addr)
	if err != nil {
		a.Error = errKind(err)
		return a
	}
	a.Rcode = dns.RcodeToString[resp.Rcode]
	if resp.Rcode != dns.RcodeSuccess && resp.Rcode != dns.RcodeNameError {
		a.Error = ErrFailed
		return a
	}
	ttl := -1
	for _, rr := range resp.Answer {
		var ip net.IP
		switch v := rr.(type) {
		case *dns.A:
			ip = v.A
		case *dns.AAAA:
			ip = v.AAAA
		default:
			continue // z. B. CNAME-Kette
		}
		if rr.Header().Rrtype != qtype {
			continue
		}
		a.Values = append(a.Values, ip.String())
		if t := int(rr.Header().Ttl); ttl < 0 || t < ttl {
			ttl = t
		}
	}
	if len(a.Values) == 0 {
		// Negative Antwort: Cache-Zeit aus dem SOA im Authority-Abschnitt
		for _, rr := range resp.Ns {
			if soa, ok := rr.(*dns.SOA); ok {
				ttl = int(min(soa.Minttl, soa.Hdr.Ttl))
			}
		}
	}
	a.TTL = max(ttl, 0)
	slices.Sort(a.Values)
	if proxied {
		a.Match = len(a.Values) > 0
	} else {
		a.Match = len(a.Values) == 1 && a.Values[0] == expected
	}
	return a
}

// nameservers ermittelt die autoritativen Server der Zone (höchstens zwei,
// eine Stunde zwischengespeichert).
func (c *Checker) nameservers(ctx context.Context, zone string) ([]Resolver, error) {
	c.mu.Lock()
	e, ok := c.nsCache[zone]
	c.mu.Unlock()
	if ok && c.now().Before(e.until) {
		return e.servers, nil
	}
	if len(c.resolvers) == 0 {
		return nil, errors.New("no resolver for NS lookup")
	}
	via := c.resolvers[0].Addr

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(zone), dns.TypeNS)
	resp, err := c.exchange(ctx, m, via)
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, rr := range resp.Answer {
		if ns, ok := rr.(*dns.NS); ok {
			hosts = append(hosts, ns.Ns)
		}
	}
	slices.Sort(hosts)
	if len(hosts) > 2 {
		hosts = hosts[:2]
	}
	var servers []Resolver
	for _, h := range hosts {
		m := new(dns.Msg)
		m.SetQuestion(h, dns.TypeA)
		resp, err := c.exchange(ctx, m, via)
		if err != nil {
			continue
		}
		for _, rr := range resp.Answer {
			if a, ok := rr.(*dns.A); ok {
				servers = append(servers, Resolver{Name: strings.TrimSuffix(h, "."),
					Addr: net.JoinHostPort(a.A.String(), c.nsPort)})
				break
			}
		}
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("no nameserver found for %s", zone)
	}
	c.mu.Lock()
	c.nsCache[zone] = nsEntry{servers: servers, until: c.now().Add(time.Hour)}
	c.mu.Unlock()
	return servers, nil
}

func errKind(err error) string {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return ErrTimeout
	}
	return ErrFailed
}

func canonicalIP(s string) string {
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap().String()
	}
	return s
}
