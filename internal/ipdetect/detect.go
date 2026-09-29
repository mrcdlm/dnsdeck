package ipdetect

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

const maxBody = 4 << 10

// Observation ist die Antwort einer einzelnen Quelle.
type Observation struct {
	Source string `json:"source"`
	IP     string `json:"ip,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Detector fragt alle Quellen einer Adressfamilie parallel ab.
type Detector struct {
	Sources []Source
	Timeout time.Duration
	// Client liefert den HTTP-Client je Familie. nil = Client mit erzwungenem
	// tcp4/tcp6-Dialer (in Tests austauschbar).
	Client func(Family) *http.Client

	once    sync.Once
	clients map[Family]*http.Client
}

func NewDetector(sources []Source) *Detector {
	return &Detector{Sources: sources, Timeout: 10 * time.Second}
}

func (d *Detector) client(f Family) *http.Client {
	if d.Client != nil {
		return d.Client(f)
	}
	d.once.Do(func() {
		d.clients = map[Family]*http.Client{
			IPv4: familyClient("tcp4"),
			IPv6: familyClient("tcp6"),
		}
	})
	return d.clients[f]
}

func familyClient(network string) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}
	return &http.Client{Transport: tr}
}

// Observe fragt alle Quellen der Familie ab und liefert gültige Adressen
// sowie alle Einzelbeobachtungen (auch Fehler) für Diagnosezwecke.
func (d *Detector) Observe(ctx context.Context, f Family) ([]netip.Addr, []Observation) {
	var srcs []Source
	for _, s := range d.Sources {
		if s.Family == f {
			srcs = append(srcs, s)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()

	obs := make([]Observation, len(srcs))
	addrs := make([]netip.Addr, len(srcs))
	var wg sync.WaitGroup
	for i, s := range srcs {
		wg.Go(func() {
			a, err := d.fetch(ctx, s)
			obs[i] = Observation{Source: s.Name}
			if err != nil {
				obs[i].Error = err.Error()
				return
			}
			obs[i].IP = a.String()
			addrs[i] = a
		})
	}
	wg.Wait()

	var valid []netip.Addr
	for _, a := range addrs {
		if a.IsValid() {
			valid = append(valid, a)
		}
	}
	return valid, obs
}

func (d *Detector) fetch(ctx context.Context, s Source) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return netip.Addr{}, err
	}
	req.Header.Set("User-Agent", "dnsdeck")
	resp, err := d.client(s.Family).Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return netip.Addr{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return netip.Addr{}, err
	}
	raw, err := s.Parse(body)
	if err != nil {
		return netip.Addr{}, err
	}
	return Validate(raw, s.Family)
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// Validate prüft, ob raw eine plausible öffentliche Adresse der Familie ist.
func Validate(raw string, f Family) (netip.Addr, error) {
	a, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("keine IP-Adresse: %q", truncate(raw, 64))
	}
	a = a.WithZone("")
	switch f {
	case IPv4:
		if !a.Is4() {
			return netip.Addr{}, fmt.Errorf("keine IPv4-Adresse: %s", a)
		}
	case IPv6:
		if !a.Is6() || a.Is4In6() {
			return netip.Addr{}, fmt.Errorf("keine IPv6-Adresse: %s", a)
		}
	}
	if !a.IsGlobalUnicast() || a.IsPrivate() || cgnat.Contains(a) {
		return netip.Addr{}, fmt.Errorf("keine öffentliche Adresse: %s", a)
	}
	return a, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
