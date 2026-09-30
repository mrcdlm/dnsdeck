package i18n

import (
	"errors"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var placeholderRe = regexp.MustCompile(`\{[a-z_]+\}`)

// Jeder Eintrag hat beide Sprachen und dieselben Platzhalter.
func TestCatalogComplete(t *testing.T) {
	for code, e := range catalog {
		if e.de == "" || e.en == "" {
			t.Errorf("%s: Übersetzung fehlt", code)
		}
		de, en := placeholderRe.FindAllString(e.de, -1), placeholderRe.FindAllString(e.en, -1)
		if strings.Join(sorted(de), ",") != strings.Join(sorted(en), ",") {
			t.Errorf("%s: Platzhalter unterschiedlich: %v vs. %v", code, de, en)
		}
	}
}

func sorted(s []string) []string {
	out := append([]string{}, s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestRender(t *testing.T) {
	m := M("ddns.adopted", "proxy", Ref("state.on"), "ttl", "Auto")
	if got := T(DE, m); got != "bestehenden Eintrag übernommen (Proxy an, TTL Auto)" {
		t.Fatalf("de: %q", got)
	}
	if got := T(EN, m); got != "adopted existing record (proxy on, TTL Auto)" {
		t.Fatalf("en: %q", got)
	}

	nested := M("ddns.recovered_after", "detail", Nest(M("cf.api_http", "status", "500")))
	if got := T(DE, nested); got != "Abgleich wieder erfolgreich (vorher: Cloudflare-API: HTTP 500)" {
		t.Fatalf("nested: %q", got)
	}

	j := JoinMsgs(M("ddns.adopted_short"), M("ddns.proxy_change", "from", Ref("state.off"), "to", Ref("state.on")))
	if got := T(EN, j); got != "adopted existing record; Proxy: off → on" {
		t.Fatalf("join: %q", got)
	}
	if !Equal(Decode(Encode(j)), j) || !Decode("").IsZero() || Encode(Msg{}) != "" {
		t.Fatal("Encode/Decode")
	}
	if JoinMsgs(M("ip.save_failed")).Code != "ip.save_failed" || !JoinMsgs().IsZero() {
		t.Fatal("JoinMsgs mit 0/1 Meldung")
	}
	if T(EN, M("gibtsnicht")) != "gibtsnicht" {
		t.Fatal("unbekannter Code")
	}
}

type locErr struct{}

func (locErr) Error() string     { return "cf" }
func (locErr) LocalizedMsg() Msg { return M("cf.api_http", "status", "403") }

func TestErrors(t *testing.T) {
	err := Wrap(E("webhook.env_missing", "name", "WEBHOOK_X"), "webhook.in_url")
	if err.Error() != "URL: environment variable WEBHOOK_X is not set" {
		t.Fatalf("Error(): %q", err.Error())
	}
	if got := T(DE, FromError(err)); got != "URL: Env-Variable WEBHOOK_X ist nicht gesetzt" {
		t.Fatalf("de: %q", got)
	}
	if got := T(DE, FromError(errors.Join(locErr{}))); got != "Cloudflare-API: HTTP 403" {
		t.Fatalf("Localizer: %q", got)
	}
	if got := T(DE, FromError(errors.New("dial tcp: timeout"))); got != "dial tcp: timeout" {
		t.Fatalf("roh: %q", got)
	}
}

func TestFromRequest(t *testing.T) {
	for header, want := range map[string]Lang{"de-DE,de;q=0.9": DE, "en-US": EN, "": EN, "fr": EN, "DE": DE} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept-Language", header)
		if got := FromRequest(r); got != want {
			t.Errorf("%q → %s", header, got)
		}
	}
}

func TestParseLegacy(t *testing.T) {
	for text, en := range map[string]string{
		"bestehenden Eintrag übernommen (Proxy aus, TTL 1 h)":                                        "adopted existing record (proxy off, TTL 1 h)",
		"bestehenden Eintrag übernommen (Proxy an, TTL Auto)":                                        "adopted existing record (proxy on, TTL Auto)",
		"bei Cloudflare angelegt":                                                                    "created at Cloudflare",
		"bestehenden Eintrag übernommen; Proxy: aus → an":                                            "adopted existing record; Proxy: off → on",
		"Proxy: an → aus; TTL: Auto → 5 min":                                                         "Proxy: on → off; TTL: Auto → 5 min",
		"Abgleich wieder erfolgreich":                                                                "Sync successful again",
		"Abgleich wieder erfolgreich (vorher: Cloudflare-API: HTTP 500: simulierter Fehler (10001))": "Sync successful again (previously: Cloudflare API: HTTP 500: simulierter Fehler (10001))",
		"keine öffentliche IPv6-Adresse bekannt":                                                     "no public IPv6 address known",
	} {
		m, ok := ParseLegacy(text)
		if !ok {
			t.Errorf("%q: not recognized", text)
			continue
		}
		if got := T(EN, m); got != en {
			t.Errorf("%q: got %q, want %q", text, got, en)
		}
	}
	for _, text := range []string{"", "irgendein anderer Text", "dial tcp: connection refused"} {
		if _, ok := ParseLegacy(text); ok {
			t.Errorf("%q: should not be recognized", text)
		}
	}
}
