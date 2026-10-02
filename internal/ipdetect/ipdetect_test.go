package ipdetect

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/dnsbl"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/isp"
	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

func addrs(ss ...string) []netip.Addr {
	var out []netip.Addr
	for _, s := range ss {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

func TestDecide(t *testing.T) {
	a, b := "203.0.113.1", "203.0.113.2"
	known := netip.MustParseAddr(a)
	tests := []struct {
		name  string
		in    []netip.Addr
		known netip.Addr
		want  string // "" = nicht bestätigt
	}{
		{"keine Antworten", nil, known, ""},
		{"einstimmig", addrs(b, b, b), known, b},
		{"Mehrheit mit Ausreißer", addrs(b, a, b), known, b},
		{"zwei gleiche", addrs(b, b), known, b},
		{"Patt", addrs(a, b), known, ""},
		{"drei verschiedene", addrs(a, b, "203.0.113.3"), known, ""},
		{"einzelne Quelle, nichts bekannt", addrs(b), netip.Addr{}, b},
		{"einzelne Quelle bestätigt Bekanntes", addrs(a), known, a},
		{"einzelne Quelle meldet Wechsel", addrs(b), known, ""},
		{"2 von 4 ist keine Mehrheit", addrs(a, a, b, "203.0.113.3"), known, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide(tt.in, tt.known)
			got := ""
			if d.Confirmed() {
				got = d.IP.String()
			} else if d.Reason.IsZero() {
				t.Error("Begründung fehlt")
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q (%+v)", got, tt.want, d)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	ok := []struct {
		raw string
		f   Family
	}{
		{"203.0.113.7", IPv4},
		{"2001:db8::1", IPv6},
		{"8.8.8.8", IPv4},
	}
	for _, c := range ok {
		if _, err := Validate(c.raw, c.f); err != nil {
			t.Errorf("%s: %v", c.raw, err)
		}
	}
	bad := []struct {
		raw string
		f   Family
	}{
		{"<html>", IPv4},
		{"192.168.1.1", IPv4},
		{"10.0.0.1", IPv4},
		{"127.0.0.1", IPv4},
		{"100.64.1.1", IPv4},
		{"2001:db8::1", IPv4},
		{"203.0.113.7", IPv6},
		{"::ffff:203.0.113.7", IPv6},
		{"fe80::1", IPv6},
		{"fd00::1", IPv6},
		{"::1", IPv6},
	}
	for _, c := range bad {
		if _, err := Validate(c.raw, c.f); err == nil {
			t.Errorf("%s (%s): Fehler erwartet", c.raw, c.f)
		}
	}
}

func TestParseTrace(t *testing.T) {
	ip, err := parseTrace([]byte("fl=1\nh=1.1.1.1\nip=203.0.113.9\nts=1\n"))
	if err != nil || ip != "203.0.113.9" {
		t.Fatalf("%q %v", ip, err)
	}
	if _, err := parseTrace([]byte("fl=1\n")); err == nil {
		t.Fatal("Fehler erwartet")
	}
}

func TestSelectSources(t *testing.T) {
	all := DefaultSources()
	got := SelectSources(all, []string{"ipify", "unbekannt"})
	if len(got) != 2 || got[0].Name != "ipify" {
		t.Fatalf("unerwartet: %+v", got)
	}
	if len(SelectSources(all, []string{"unbekannt"})) != len(all) {
		t.Fatal("Fallback auf alle Quellen erwartet")
	}
}

func fakeServer(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestDetectorObserve(t *testing.T) {
	good := fakeServer(t, 200, "203.0.113.5\n")
	trace := fakeServer(t, 200, "h=x\nip=203.0.113.5\n")
	broken := fakeServer(t, 500, "")
	private := fakeServer(t, 200, "192.168.0.10")
	v6 := fakeServer(t, 200, "2001:db8::5")

	d := NewDetector([]Source{
		{Name: "a", Family: IPv4, URL: good, Parse: parsePlain},
		{Name: "b", Family: IPv4, URL: trace, Parse: parseTrace},
		{Name: "c", Family: IPv4, URL: broken, Parse: parsePlain},
		{Name: "d", Family: IPv4, URL: private, Parse: parsePlain},
		{Name: "e", Family: IPv6, URL: v6, Parse: parsePlain},
	})
	d.Client = func(Family) *http.Client { return http.DefaultClient }

	got, obs := d.Observe(context.Background(), IPv4)
	if len(got) != 2 || len(obs) != 4 {
		t.Fatalf("got %v, obs %+v", got, obs)
	}
	if obs[2].ErrorMsg.IsZero() || obs[3].ErrorMsg.IsZero() {
		t.Fatalf("Fehler für c/d erwartet: %+v", obs)
	}
	if dec := Decide(got, netip.Addr{}); dec.IP.String() != "203.0.113.5" {
		t.Fatalf("Decide: %+v", dec)
	}

	got, _ = d.Observe(context.Background(), IPv6)
	if len(got) != 1 || got[0].String() != "2001:db8::5" {
		t.Fatalf("v6: %v", got)
	}
}

// --- Tracker ---

type fakeObserver map[Family][]netip.Addr

func (f fakeObserver) Observe(_ context.Context, fam Family) ([]netip.Addr, []Observation) {
	var obs []Observation
	for _, a := range f[fam] {
		obs = append(obs, Observation{Source: "fake", IP: a.String()})
	}
	return f[fam], obs
}

type memStore struct {
	changes []store.IPChange
	fail    bool
}

func (m *memStore) LatestIPChange(_ context.Context, fam string) (store.IPChange, error) {
	for i := len(m.changes) - 1; i >= 0; i-- {
		if m.changes[i].Family == fam {
			return m.changes[i], nil
		}
	}
	return store.IPChange{}, store.ErrNotFound
}

func (m *memStore) InsertIPChange(_ context.Context, c store.IPChange) (int64, error) {
	if m.fail {
		return 0, errors.New("db kaputt")
	}
	m.changes = append(m.changes, c)
	return int64(len(m.changes)), nil
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestTracker(t *testing.T) {
	ctx := context.Background()
	st := &memStore{}
	obs := fakeObserver{IPv4: addrs("203.0.113.1", "203.0.113.1")}
	tr := NewTracker(obs, st, discard())
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return clock }

	s, err := tr.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.IPv4.IP != "203.0.113.1" || s.IPv4.Status != StatusOK || len(st.changes) != 1 {
		t.Fatalf("erster Check: %+v / %+v", s.IPv4, st.changes)
	}
	if s.IPv6.Status != StatusUnavailable || s.IPv6.IP != "" {
		t.Fatalf("v6: %+v", s.IPv6)
	}

	// gleiche IP → kein neuer Eintrag, "since" bleibt
	clock = clock.Add(time.Hour)
	s, _ = tr.Check(ctx)
	if len(st.changes) != 1 || !s.IPv4.Since.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unnötiger Eintrag: %+v", st.changes)
	}

	// uneinige Quellen → alte IP bleibt, Status unconfirmed
	obs[IPv4] = addrs("203.0.113.2", "203.0.113.3")
	s, _ = tr.Check(ctx)
	if s.IPv4.IP != "203.0.113.1" || s.IPv4.Status != StatusUnconfirmed || len(st.changes) != 1 {
		t.Fatalf("uneinig: %+v", s.IPv4)
	}

	// bestätigter Wechsel
	obs[IPv4] = addrs("203.0.113.2", "203.0.113.2", "203.0.113.3")
	s, _ = tr.Check(ctx)
	last := st.changes[len(st.changes)-1]
	if s.IPv4.IP != "203.0.113.2" || last.PreviousIP != "203.0.113.1" || !s.IPv4.Since.Equal(clock) {
		t.Fatalf("Wechsel: %+v / %+v", s.IPv4, last)
	}

	// Neustart: Load übernimmt die letzte IP; einzelne abweichende Quelle
	// löst keinen Wechsel aus
	tr2 := NewTracker(fakeObserver{IPv4: addrs("203.0.113.9")}, st, discard())
	if err := tr2.Load(ctx); err != nil {
		t.Fatal(err)
	}
	s, _ = tr2.Check(ctx)
	if s.IPv4.IP != "203.0.113.2" || s.IPv4.Status != StatusUnconfirmed {
		t.Fatalf("nach Neustart: %+v", s.IPv4)
	}
}

func TestTrackerPersistError(t *testing.T) {
	st := &memStore{fail: true}
	tr := NewTracker(fakeObserver{IPv4: addrs("203.0.113.1", "203.0.113.1")}, st, discard())
	s, err := tr.Check(context.Background())
	if err == nil || s.IPv4.IP != "" || s.IPv4.Status != StatusUnconfirmed {
		t.Fatalf("err=%v state=%+v", err, s.IPv4)
	}
	// nach Behebung wird der Wechsel nachgeholt
	st.fail = false
	s, err = tr.Check(context.Background())
	if err != nil || s.IPv4.IP != "203.0.113.1" || len(st.changes) != 1 {
		t.Fatalf("err=%v state=%+v", err, s.IPv4)
	}
}

func TestLocalize(t *testing.T) {
	s := State{IPv4: FamilyState{MessageMsg: Decide(nil, netip.Addr{}).Reason,
		Sources: []Observation{{Source: "x", ErrorMsg: i18nErr(t)}}}}
	en, de := s.Localize("en"), s.Localize("de")
	if en.IPv4.Message != "no source reachable" || de.IPv4.Message != "keine Quelle erreichbar" {
		t.Fatalf("Message: %q / %q", en.IPv4.Message, de.IPv4.Message)
	}
	if de.IPv4.Sources[0].Error != "keine öffentliche Adresse: 192.168.1.1" || s.IPv4.Sources[0].Error != "" {
		t.Fatalf("Source-Fehler: %+v / Original verändert: %+v", de.IPv4.Sources, s.IPv4.Sources)
	}
}

func i18nErr(t *testing.T) i18n.Msg {
	t.Helper()
	_, err := Validate("192.168.1.1", IPv4)
	return i18n.FromError(err)
}

type fakeISP struct {
	info  map[string]isp.Info
	calls int
}

func (f *fakeISP) Lookup(_ context.Context, a netip.Addr) (isp.Info, error) {
	f.calls++
	if i, ok := f.info[a.String()]; ok {
		return i, nil
	}
	return isp.Info{}, isp.ErrNotFound
}

func TestTrackerISP(t *testing.T) {
	ctx := context.Background()
	obs := fakeObserver{IPv4: addrs("203.0.113.1", "203.0.113.1")}
	lookup := &fakeISP{info: map[string]isp.Info{"203.0.113.1": {ASN: 64500, Name: "Example ISP"}}}
	tr := NewTracker(obs, &memStore{}, discard())
	tr.ISP = lookup
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return clock }

	s, _ := tr.Check(ctx)
	if s.IPv4.ISP == nil || s.IPv4.ISP.ASN != 64500 || s.IPv6.ISP != nil || lookup.calls != 1 {
		t.Fatalf("erster Check: %+v / %d Abfragen", s.IPv4.ISP, lookup.calls)
	}

	// gleiche IP innerhalb der Gültigkeit → keine neue Abfrage
	clock = clock.Add(time.Hour)
	if s, _ = tr.Check(ctx); s.IPv4.ISP == nil || lookup.calls != 1 {
		t.Fatalf("Cache: %+v / %d Abfragen", s.IPv4.ISP, lookup.calls)
	}

	// abgelaufen und Abfrage scheitert → altes Ergebnis bleibt
	clock = clock.Add(ispTTL)
	delete(lookup.info, "203.0.113.1")
	if s, _ = tr.Check(ctx); s.IPv4.ISP == nil || lookup.calls != 2 {
		t.Fatalf("Fehler bei gleicher IP: %+v / %d Abfragen", s.IPv4.ISP, lookup.calls)
	}

	// IP-Wechsel, Anbieter unbekannt → keine veraltete Anzeige
	obs[IPv4] = addrs("203.0.113.2", "203.0.113.2")
	if s, _ = tr.Check(ctx); s.IPv4.ISP != nil || lookup.calls != 3 {
		t.Fatalf("Wechsel: %+v / %d Abfragen", s.IPv4.ISP, lookup.calls)
	}

	// erneuter Versuch erst nach ispRetryTTL
	lookup.info["203.0.113.2"] = isp.Info{ASN: 64501}
	if s, _ = tr.Check(ctx); s.IPv4.ISP != nil || lookup.calls != 3 {
		t.Fatalf("zu früh erneut: %d Abfragen", lookup.calls)
	}
	clock = clock.Add(ispRetryTTL)
	if s, _ = tr.Check(ctx); s.IPv4.ISP == nil || s.IPv4.ISP.ASN != 64501 {
		t.Fatalf("nach Wartezeit: %+v", s.IPv4.ISP)
	}
}

type fakeBlocklist struct {
	listed map[string][]string // IP → gelistete Zonen
	down   bool                // keine Liste erreichbar
	calls  int
}

func (f *fakeBlocklist) Check(_ context.Context, a netip.Addr) dnsbl.Result {
	f.calls++
	if f.down {
		return dnsbl.Result{Status: dnsbl.StatusUnknown, Lists: []dnsbl.Entry{{Zone: "a.example", Status: dnsbl.StatusError}}}
	}
	res := dnsbl.Result{Status: dnsbl.StatusClean}
	for _, z := range []string{"a.example", "b.example"} {
		e := dnsbl.Entry{Name: strings.ToUpper(z[:1]), Zone: z, Status: dnsbl.StatusClean}
		if slices.Contains(f.listed[a.String()], z) {
			e.Status, res.Status = dnsbl.StatusListed, dnsbl.StatusListed
		}
		res.Lists = append(res.Lists, e)
	}
	return res
}

type eventLog []notify.Event

func (l *eventLog) Notify(ev notify.Event) { *l = append(*l, ev) }

func TestTrackerBlocklist(t *testing.T) {
	ctx := context.Background()
	obs := fakeObserver{IPv4: addrs("203.0.113.1", "203.0.113.1"), IPv6: addrs("2001:db8::1", "2001:db8::1")}
	bl := &fakeBlocklist{listed: map[string][]string{}}
	var evs eventLog
	tr := NewTracker(obs, &memStore{}, discard())
	tr.Blocklist, tr.Notifier = bl, &evs
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return clock }

	s, _ := tr.Check(ctx)
	if s.IPv4.Blocklist == nil || s.IPv4.Blocklist.Status != dnsbl.StatusClean || s.IPv6.Blocklist != nil ||
		bl.calls != 1 || len(evs) != 0 {
		t.Fatalf("erster Check: %+v / %d Abfragen / %d Ereignisse", s.IPv4.Blocklist, bl.calls, len(evs))
	}

	// innerhalb der Gültigkeit keine neue Abfrage
	clock = clock.Add(time.Hour)
	if tr.Check(ctx); bl.calls != 1 {
		t.Fatalf("Cache: %d Abfragen", bl.calls)
	}

	// nach Ablauf neu gelistet → genau eine Benachrichtigung
	clock = clock.Add(blocklistTTL)
	bl.listed["203.0.113.1"] = []string{"b.example"}
	s, _ = tr.Check(ctx)
	if s.IPv4.Blocklist.Status != dnsbl.StatusListed || len(evs) != 1 ||
		evs[0].Type != notify.EventBlocklisted || evs[0].Data["zones"] != "b.example" || evs[0].Data["lists"] != "B" {
		t.Fatalf("Listung: %+v / %+v", s.IPv4.Blocklist, evs)
	}

	// weiterhin gelistet → keine erneute Meldung; zusätzliche Liste → nur diese
	clock = clock.Add(blocklistTTL)
	tr.Check(ctx)
	bl.listed["203.0.113.1"] = []string{"a.example", "b.example"}
	clock = clock.Add(blocklistTTL)
	tr.Check(ctx)
	if len(evs) != 2 || evs[1].Data["zones"] != "a.example" {
		t.Fatalf("Folgemeldungen: %+v", evs)
	}

	// Listen nicht erreichbar → letztes Ergebnis bleibt, früher erneut versuchen
	bl.down = true
	clock = clock.Add(blocklistTTL)
	if s, _ = tr.Check(ctx); s.IPv4.Blocklist.Status != dnsbl.StatusListed {
		t.Fatalf("Ausfall: %+v", s.IPv4.Blocklist)
	}
	calls := bl.calls
	clock = clock.Add(blocklistRetryTTL)
	if tr.Check(ctx); bl.calls != calls+1 {
		t.Fatalf("Wiederholung: %d → %d", calls, bl.calls)
	}

	// IP-Wechsel → sofort neu prüfen, neue IP sauber
	bl.down = false
	obs[IPv4] = addrs("203.0.113.2", "203.0.113.2")
	if s, _ = tr.Check(ctx); s.IPv4.Blocklist == nil || s.IPv4.Blocklist.Status != dnsbl.StatusClean {
		t.Fatalf("Wechsel: %+v", s.IPv4.Blocklist)
	}
}
