package dnsbl

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

type fakeResolver struct {
	answers map[string][]string
	errs    map[string]error
	seen    []string
}

func (f *fakeResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	f.seen = append(f.seen, host)
	if err, ok := f.errs[host]; ok {
		return nil, err
	}
	if v, ok := f.answers[host]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

var testLists = []List{
	{Name: "Spamhaus ZEN", Zone: "zen.spamhaus.org"},
	{Name: "SpamCop", Zone: "bl.spamcop.net"},
}

func check(t *testing.T, r *fakeResolver, ip string) Result {
	t.Helper()
	c := &Checker{Resolver: r, Lists: testLists, Timeout: time.Second,
		now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	return c.Check(t.Context(), netip.MustParseAddr(ip))
}

func TestQueryName(t *testing.T) {
	if got := queryName(netip.MustParseAddr("203.0.113.7"), "zen.spamhaus.org"); got != "7.113.0.203.zen.spamhaus.org" {
		t.Fatal(got)
	}
}

func TestClean(t *testing.T) {
	r := &fakeResolver{}
	res := check(t, r, "203.0.113.7")
	if res.Status != StatusClean || len(res.Lists) != 2 || res.Lists[0].Status != StatusClean ||
		res.Lists[1].Status != StatusClean || len(res.Listed()) != 0 {
		t.Fatalf("%+v", res)
	}
	if len(r.seen) != 2 {
		t.Fatalf("Abfragen: %v", r.seen)
	}
}

func TestListed(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{
		"7.113.0.203.zen.spamhaus.org": {"127.0.0.10", "127.0.0.4", "127.0.0.2"},
		"7.113.0.203.bl.spamcop.net":   {"127.0.0.2"},
	}}
	res := check(t, r, "203.0.113.7")
	want := []Entry{
		{Name: "Spamhaus ZEN", Zone: "zen.spamhaus.org", Status: StatusListed,
			Codes: []string{"127.0.0.2", "127.0.0.4", "127.0.0.10"}, Detail: "SBL, XBL, PBL"},
		{Name: "SpamCop", Zone: "bl.spamcop.net", Status: StatusListed, Codes: []string{"127.0.0.2"}},
	}
	if res.Status != StatusListed || !reflect.DeepEqual(res.Lists, want) || len(res.Listed()) != 2 {
		t.Fatalf("%+v", res)
	}
}

// Die Policy Block List führt nahezu alle Endkundenanschlüsse – kein Alarm.
func TestPBLOnlyIsPolicy(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{"7.113.0.203.zen.spamhaus.org": {"127.0.0.11", "127.0.0.10"}}}
	res := check(t, r, "203.0.113.7")
	if res.Status != StatusClean || res.Lists[0].Status != StatusPolicy || res.Lists[0].Detail != "PBL" ||
		len(res.Listed()) != 0 {
		t.Fatalf("%+v", res)
	}
}

// Spamhaus lehnt Abfragen über öffentliche Resolver mit 127.255.255.254 ab.
func TestRefusedIsNotListed(t *testing.T) {
	r := &fakeResolver{
		answers: map[string][]string{"7.113.0.203.zen.spamhaus.org": {"127.255.255.254"}},
		errs:    map[string]error{"7.113.0.203.bl.spamcop.net": errors.New("i/o timeout")},
	}
	res := check(t, r, "203.0.113.7")
	if res.Status != StatusUnknown || res.Lists[0].Status != StatusRefused || res.Lists[1].Status != StatusError {
		t.Fatalf("%+v", res)
	}
}

func TestNonLoopbackAnswerIsError(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{"7.113.0.203.bl.spamcop.net": {"198.51.100.1"}}}
	res := check(t, r, "203.0.113.7")
	if res.Lists[1].Status != StatusError || res.Status != StatusClean {
		t.Fatalf("%+v", res)
	}
}

func TestIPv6NotChecked(t *testing.T) {
	r := &fakeResolver{}
	res := check(t, r, "2001:db8::1")
	if res.Status != StatusUnknown || len(res.Lists) != 0 || len(r.seen) != 0 {
		t.Fatalf("%+v %v", res, r.seen)
	}
}

func TestParseLists(t *testing.T) {
	if l, err := ParseLists(""); err != nil || !reflect.DeepEqual(l, DefaultLists) {
		t.Fatalf("Standard: %v %v", l, err)
	}
	if l, err := ParseLists(" OFF "); err != nil || l != nil {
		t.Fatalf("off: %v %v", l, err)
	}
	l, err := ParseLists("Spamhaus=zen.spamhaus.org., bl.spamcop.net")
	want := []List{{Name: "Spamhaus", Zone: "zen.spamhaus.org"}, {Name: "bl.spamcop.net", Zone: "bl.spamcop.net"}}
	if err != nil || !reflect.DeepEqual(l, want) {
		t.Fatalf("%v %v", l, err)
	}
	for _, bad := range []string{",", "localhost", "x=bad zone.org", "a..b"} {
		if _, err := ParseLists(bad); err == nil {
			t.Errorf("%q: Fehler erwartet", bad)
		}
	}
}
