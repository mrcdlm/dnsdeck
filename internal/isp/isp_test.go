package isp

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

type fakeResolver struct {
	txt  map[string][]string
	ptr  map[string][]string
	seen []string
}

func (f *fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.seen = append(f.seen, name)
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (f *fakeResolver) LookupAddr(_ context.Context, addr string) ([]string, error) {
	if v, ok := f.ptr[addr]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: addr, IsNotFound: true}
}

func TestOriginName(t *testing.T) {
	cases := map[string]string{
		"203.0.113.7": "7.113.0.203.origin.asn.cymru.com",
		"2001:db8::1": "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.origin6.asn.cymru.com",
	}
	for in, want := range cases {
		if got := originName(netip.MustParseAddr(in)); got != want {
			t.Errorf("originName(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestLookupIPv4(t *testing.T) {
	r := &fakeResolver{
		txt: map[string][]string{
			"7.113.0.203.origin.asn.cymru.com": {
				"3320 | 203.0.0.0/8 | DE | ripencc | 2001-01-01",
				"3320 6805 | 203.0.113.0/24 | de | ripencc | 2007-07-03",
			},
			"AS3320.asn.cymru.com": {"3320 | DE | ripencc | 1993-02-10 | DTAG Internet service provider operations, DE"},
		},
		ptr: map[string][]string{"203.0.113.7": {"p5dd8a0b7.dip0.t-ipconnect.de."}},
	}
	got, err := (&Lookup{Resolver: r, Timeout: time.Second}).Lookup(t.Context(), netip.MustParseAddr("203.0.113.7"))
	if err != nil {
		t.Fatal(err)
	}
	want := Info{ASN: 3320, Name: "DTAG Internet service provider operations", Prefix: "203.0.113.0/24",
		Country: "DE", Registry: "ripencc", Hostname: "p5dd8a0b7.dip0.t-ipconnect.de"}
	if got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestLookupNotFound(t *testing.T) {
	r := &fakeResolver{ptr: map[string][]string{"2001:db8::1": {"host.example."}}}
	got, err := (&Lookup{Resolver: r, Timeout: time.Second}).Lookup(t.Context(), netip.MustParseAddr("2001:db8::1"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got.Hostname != "host.example" {
		t.Errorf("hostname = %q", got.Hostname)
	}
}

func TestLookupWithoutASName(t *testing.T) {
	r := &fakeResolver{txt: map[string][]string{
		"7.113.0.203.origin.asn.cymru.com": {"64500 | 203.0.113.0/24 | NL | ripencc |"},
	}}
	got, err := (&Lookup{Resolver: r, Timeout: time.Second}).Lookup(t.Context(), netip.MustParseAddr("203.0.113.7"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ASN != 64500 || got.Name != "" || got.Hostname != "" || got.Country != "NL" {
		t.Errorf("got %+v", got)
	}
}

func TestParseASName(t *testing.T) {
	cases := map[string]string{
		"3209 | DE | ripencc | 1994-06-15 | VODANET International IP-Backbone of Vodafone, DE": "VODANET International IP-Backbone of Vodafone",
		"13335 | US | arin | 2010-07-14 | CLOUDFLARENET, US":                                   "CLOUDFLARENET",
		"64500 | DE | ripencc | 2000-01-01 | Example, Inc":                                     "Example, Inc",
		"garbage": "",
	}
	for in, want := range cases {
		if got := parseASName(in); got != want {
			t.Errorf("parseASName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseOriginInvalid(t *testing.T) {
	for _, in := range []string{"", "x | 1.2.3.0/24", "3320 | kein-netz", "0 | 1.2.3.0/24"} {
		if _, _, ok := parseOrigin(in); ok {
			t.Errorf("parseOrigin(%q) ok, want invalid", in)
		}
	}
}
