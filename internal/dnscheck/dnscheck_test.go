package dnscheck

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

// zone ist ein kleiner DNS-Server für Tests.
type zone struct {
	mu      sync.Mutex
	records map[string][]string // "name. A" → IPs
	ttl     uint32
	silent  bool // nie antworten (Zeitlimit)
	queries int
}

func (z *zone) set(name, typ string, ips ...string) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.records[dns.Fqdn(name)+" "+typ] = ips
}

func (z *zone) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.queries++
	if z.silent {
		return
	}
	q := req.Question[0]
	m := new(dns.Msg)
	m.SetReply(req)
	typ := dns.TypeToString[q.Qtype]
	ips, ok := z.records[q.Name+" "+typ]
	for _, ip := range ips {
		hdr := dns.RR_Header{Name: q.Name, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: z.ttl}
		if typ == "NS" {
			m.Answer = append(m.Answer, &dns.NS{Hdr: hdr, Ns: ip})
		} else if q.Qtype == dns.TypeA {
			m.Answer = append(m.Answer, &dns.A{Hdr: hdr, A: net.ParseIP(ip)})
		} else {
			m.Answer = append(m.Answer, &dns.AAAA{Hdr: hdr, AAAA: net.ParseIP(ip)})
		}
	}
	if !ok {
		m.Rcode = dns.RcodeNameError
		m.Ns = []dns.RR{&dns.SOA{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 1800},
			Ns: "ns1.example.com.", Mbox: "dns.example.com.", Minttl: 90}}
	}
	_ = w.WriteMsg(m)
}

func startZone(t *testing.T) (*zone, string) {
	t.Helper()
	z := &zone{records: map[string][]string{}, ttl: 300}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	srv := &dns.Server{PacketConn: pc, Handler: z, NotifyStartedFunc: func() { close(started) }}
	go func() { _ = srv.ActivateAndServe() }()
	<-started
	t.Cleanup(func() { _ = srv.Shutdown() })
	return z, pc.LocalAddr().String()
}

func newChecker(auth bool, addrs ...string) *Checker {
	var rs []Resolver
	for i, a := range addrs {
		rs = append(rs, Resolver{Name: string(rune('A' + i)), Addr: a})
	}
	c := New(rs, auth)
	c.timeout = 300 * time.Millisecond
	return c
}

func TestParseResolvers(t *testing.T) {
	if rs, err := ParseResolvers(""); err != nil || len(rs) != len(DefaultResolvers) {
		t.Fatalf("default: %v %v", rs, err)
	}
	if rs, err := ParseResolvers("off"); err != nil || rs != nil {
		t.Fatalf("off: %v %v", rs, err)
	}
	rs, err := ParseResolvers("9.9.9.9, Mock=127.0.0.1:8553, 2606:4700::1111, [::1]:5353")
	if err != nil {
		t.Fatal(err)
	}
	want := []Resolver{
		{"9.9.9.9", "9.9.9.9:53"}, {"Mock", "127.0.0.1:8553"},
		{"2606:4700::1111", "[2606:4700::1111]:53"}, {"::1", "[::1]:5353"},
	}
	if len(rs) != len(want) {
		t.Fatalf("got %v", rs)
	}
	for i := range want {
		if rs[i] != want[i] {
			t.Errorf("%d: got %v, want %v", i, rs[i], want[i])
		}
	}
	for _, bad := range []string{"dns.google", "1.1.1.1:x", ","} {
		if _, err := ParseResolvers(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestCheckStates(t *testing.T) {
	fresh, a1 := startZone(t)
	stale, a2 := startZone(t)
	fresh.set("home.example.com", "A", "203.0.113.7")
	stale.set("home.example.com", "A", "198.51.100.1")
	stale.ttl = 120
	c := newChecker(false, a1, a2)
	q := Query{Name: "home.example.com", Type: "A", Expected: "203.0.113.7"}

	res := c.Check(context.Background(), q)
	if res.Status != StatusPartial || res.Matching != 1 || res.Total != 2 || res.MaxWait != 120 {
		t.Fatalf("partial: %+v", res)
	}
	if !res.Answers[0].Match || res.Answers[1].Match || res.Answers[1].Values[0] != "198.51.100.1" {
		t.Fatalf("answers: %+v", res.Answers)
	}

	stale.set("home.example.com", "A", "203.0.113.7")
	if res := c.Check(context.Background(), q); res.Status != StatusPropagated || res.MaxWait != 0 {
		t.Fatalf("propagated: %+v", res)
	}

	// Neuer Name, noch nirgends bekannt → negative Cache-Zeit aus dem SOA
	res = c.Check(context.Background(), Query{Name: "neu.example.com", Type: "A", Expected: "203.0.113.7"})
	if res.Status != StatusPending || res.Answers[0].Rcode != "NXDOMAIN" || res.MaxWait != 90 {
		t.Fatalf("pending: %+v", res)
	}

	// Mehrere Antworten gelten nicht als "genau die erwartete IP"
	fresh.set("multi.example.com", "A", "203.0.113.7", "203.0.113.8")
	res = newChecker(false, a1).Check(context.Background(), Query{Name: "multi.example.com", Type: "A", Expected: "203.0.113.7"})
	if res.Status != StatusPending {
		t.Fatalf("multi: %+v", res)
	}
}

func TestCheckIPv6Canonical(t *testing.T) {
	z, addr := startZone(t)
	z.set("v6.example.com", "AAAA", "2001:db8::1")
	res := newChecker(false, addr).Check(context.Background(),
		Query{Name: "v6.example.com", Type: "AAAA", Expected: "2001:0db8:0:0::1"})
	if res.Status != StatusPropagated {
		t.Fatalf("%+v", res)
	}
}

func TestCheckProxiedOnlyNeedsResolution(t *testing.T) {
	z, addr := startZone(t)
	z.set("app.example.com", "A", "104.16.0.1")
	c := newChecker(false, addr)
	res := c.Check(context.Background(), Query{Name: "app.example.com", Type: "A", Expected: "203.0.113.7", Proxied: true})
	if res.Status != StatusPropagated || res.Expected != "" || !res.Proxied {
		t.Fatalf("%+v", res)
	}
	res = c.Check(context.Background(), Query{Name: "fehlt.example.com", Type: "A", Proxied: true})
	if res.Status != StatusPending {
		t.Fatalf("missing: %+v", res)
	}
}

func TestCheckTimeoutIsErrorNotMismatch(t *testing.T) {
	ok, a1 := startZone(t)
	dead, a2 := startZone(t)
	ok.set("home.example.com", "A", "203.0.113.7")
	dead.silent = true

	res := newChecker(false, a1, a2).Check(context.Background(), Query{Name: "home.example.com", Type: "A", Expected: "203.0.113.7"})
	if res.Status != StatusPropagated || res.Errors != 1 || res.Total != 1 || res.Answers[1].Error != ErrTimeout {
		t.Fatalf("%+v", res)
	}
	res = newChecker(false, a2).Check(context.Background(), Query{Name: "home.example.com", Type: "A", Expected: "203.0.113.7"})
	if res.Status != StatusError {
		t.Fatalf("all failed: %+v", res)
	}
}

func TestCheckAuthoritative(t *testing.T) {
	z, addr := startZone(t)
	_, port, _ := net.SplitHostPort(addr)
	// Derselbe Server spielt Resolver und Nameserver der Zone.
	z.set("example.com", "NS", "ns1.example.com.")
	z.set("ns1.example.com", "A", "127.0.0.1")
	z.set("home.example.com", "A", "203.0.113.7")
	c := newChecker(true, addr)
	c.nsPort = port

	q := Query{Name: "home.example.com", Type: "A", Zone: "example.com", Expected: "203.0.113.7"}
	res := c.Check(context.Background(), q)
	if res.Status != StatusPropagated || len(res.Answers) != 2 || !res.Answers[1].Authoritative ||
		res.Answers[1].Resolver != "ns1.example.com" {
		t.Fatalf("%+v", res)
	}
	// NS-Abfrage wird zwischengespeichert
	z.mu.Lock()
	before := z.queries
	z.mu.Unlock()
	c.Check(context.Background(), q)
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.queries-before != 2 {
		t.Fatalf("expected 2 queries (no NS lookup), got %d", z.queries-before)
	}
}

func TestCheckAuthoritativeLookupFails(t *testing.T) {
	_, addr := startZone(t) // kein NS-Eintrag
	res := newChecker(true, addr).Check(context.Background(), Query{Name: "x.example.com", Type: "A", Zone: "example.com"})
	last := res.Answers[len(res.Answers)-1]
	if !last.Authoritative || last.Error == "" {
		t.Fatalf("%+v", res.Answers)
	}
}

// memStore hält Records für den Scheduler-Test.
type memStore struct {
	mu   sync.Mutex
	recs map[int64]store.Record
	sets int
}

func (m *memStore) GetRecord(_ context.Context, id int64) (store.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recs[id]
	if !ok {
		return store.Record{}, store.ErrNotFound
	}
	return r, nil
}

func (m *memStore) SetRecordPropagation(_ context.Context, id int64, b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.recs[id]
	r.Propagation = b
	m.recs[id] = r
	m.sets++
	return nil
}

func (m *memStore) result(t *testing.T, id int64) Result {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	var res Result
	if err := json.Unmarshal(m.recs[id].Propagation, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSchedulerRepeatsUntilPropagated(t *testing.T) {
	z, addr := startZone(t)
	z.set("home.example.com", "A", "198.51.100.1")
	st := &memStore{recs: map[int64]store.Record{
		1: {ID: 1, Name: "home.example.com", Type: "A", CurrentIP: "203.0.113.7", TTL: 60, Enabled: true},
	}}
	s := NewScheduler(context.Background(), newChecker(false, addr), st, slog.New(slog.DiscardHandler))
	s.InitialDelay, s.Interval = 5*time.Millisecond, 20*time.Millisecond

	s.Watch(1)
	deadline := time.Now().Add(2 * time.Second)
	for {
		st.mu.Lock()
		n := st.sets
		st.mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no repeated check")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if res := st.result(t, 1); res.Status != StatusPending {
		t.Fatalf("%+v", res)
	}
	if !s.Watching(1) {
		t.Fatal("should still watch")
	}

	z.set("home.example.com", "A", "203.0.113.7")
	s.Wait()
	if res := st.result(t, 1); res.Status != StatusPropagated {
		t.Fatalf("%+v", res)
	}
	if s.Watching(1) {
		t.Fatal("watch should have ended")
	}
}

func TestSchedulerGivesUpAfterTTL(t *testing.T) {
	_, addr := startZone(t)
	st := &memStore{recs: map[int64]store.Record{
		1: {ID: 1, Name: "nie.example.com", Type: "A", CurrentIP: "203.0.113.7", TTL: 60, Enabled: true},
	}}
	s := NewScheduler(context.Background(), newChecker(false, addr), st, slog.New(slog.DiscardHandler))
	s.InitialDelay, s.Interval = time.Millisecond, 5*time.Millisecond
	s.Grace, s.MaxWatch = 0, 30*time.Millisecond // Obergrenze statt TTL

	done := make(chan struct{})
	s.Watch(1)
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop")
	}
}

func TestSchedulerWatchRestartsAndStopsOnDelete(t *testing.T) {
	_, addr := startZone(t)
	st := &memStore{recs: map[int64]store.Record{1: {ID: 1, Name: "x.example.com", Type: "A", CurrentIP: "203.0.113.7"}}}
	s := NewScheduler(context.Background(), newChecker(false, addr), st, slog.New(slog.DiscardHandler))
	s.InitialDelay, s.Interval = 20*time.Millisecond, 10*time.Millisecond
	s.Watch(1)
	s.Watch(1) // ersetzt die erste Prüfung
	st.mu.Lock()
	delete(st.recs, 1)
	st.mu.Unlock()
	s.Wait()
	if s.Watching(1) || st.sets != 0 {
		t.Fatalf("watching=%v sets=%d", s.Watching(1), st.sets)
	}
}

func TestCheckNowWithoutIP(t *testing.T) {
	st := &memStore{recs: map[int64]store.Record{1: {ID: 1, Name: "x.example.com", Type: "AAAA"}}}
	s := NewScheduler(context.Background(), newChecker(false), st, slog.New(slog.DiscardHandler))
	if _, err := s.CheckNow(context.Background(), 1); err != ErrNothingToCheck {
		t.Fatalf("got %v", err)
	}
}
