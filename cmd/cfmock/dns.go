package main

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"

	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare/cftest"
)

// Adressen, die für proxied Einträge ausgeliefert werden (wie Cloudflare-Edge).
const (
	proxyV4 = "104.16.0.1"
	proxyV6 = "2606:4700::1"
)

// dnsView beantwortet DNS-Anfragen aus den Mock-Records – entweder mit dem
// aktuellen Stand oder mit dem Stand von vor lag (wie ein Resolver-Cache).
type dnsView struct {
	fake *cftest.Fake
	lag  time.Duration

	mu        sync.Mutex
	snapshots []snapshot // aufsteigend nach Zeit
}

type snapshot struct {
	at      time.Time
	records []cftest.Record
}

// record merkt sich regelmäßig den Stand, damit der verzögerte Server den
// alten ausliefern kann.
func (v *dnsView) record() {
	for {
		now := time.Now()
		v.mu.Lock()
		v.snapshots = append(v.snapshots, snapshot{at: now, records: v.fake.Records()})
		// Stände älter als lag (bis auf den jüngsten davon) werden nicht mehr gebraucht
		cut := 0
		for i, s := range v.snapshots {
			if s.at.Before(now.Add(-v.lag)) {
				cut = i
			}
		}
		v.snapshots = v.snapshots[cut:]
		v.mu.Unlock()
		time.Sleep(250 * time.Millisecond)
	}
}

func (v *dnsView) records() []cftest.Record {
	if v.lag == 0 {
		return v.fake.Records()
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	limit := time.Now().Add(-v.lag)
	var out []cftest.Record
	for _, s := range v.snapshots {
		if s.at.After(limit) {
			break
		}
		out = s.records
	}
	return out
}

func (v *dnsView) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Authoritative = true
	q := req.Question[0]
	name := strings.TrimSuffix(strings.ToLower(q.Name), ".")
	ttl := uint32(300)
	if v.lag > 0 {
		ttl = uint32(v.lag.Seconds())
	}
	found := false
	for _, r := range v.records() {
		if r.Name != name {
			continue
		}
		found = true
		if dns.TypeToString[q.Qtype] != r.Type {
			continue
		}
		content := r.Content
		if r.Proxied {
			content = proxyV4
			if r.Type == "AAAA" {
				content = proxyV6
			}
		}
		hdr := dns.RR_Header{Name: q.Name, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: ttl}
		if r.Type == "A" {
			m.Answer = append(m.Answer, &dns.A{Hdr: hdr, A: net.ParseIP(content)})
		} else {
			m.Answer = append(m.Answer, &dns.AAAA{Hdr: hdr, AAAA: net.ParseIP(content)})
		}
	}
	if !found {
		m.Rcode = dns.RcodeNameError
	}
	if len(m.Answer) == 0 {
		m.Ns = []dns.RR{&dns.SOA{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: ttl},
			Ns: "ns.cfmock.invalid.", Mbox: "dns.cfmock.invalid.", Minttl: ttl}}
	}
	_ = w.WriteMsg(m)
}

func serveDNS(addr string, v *dnsView) {
	if v.lag > 0 {
		go v.record()
	}
	srv := &dns.Server{Addr: addr, Net: "udp", Handler: v}
	go func() { log.Fatal(srv.ListenAndServe()) }()
}
