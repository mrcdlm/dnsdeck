// Package i18n übersetzt alle Texte, die der Server an die Oberfläche oder
// in Benachrichtigungen liefert. Meldungen werden als Code + Parameter
// transportiert und gespeichert (Msg) und erst beim Ausliefern in der
// gewünschten Sprache gerendert.
package i18n

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
)

type Lang string

const (
	DE Lang = "de"
	EN Lang = "en"
)

// Parse liefert DE für "de…", sonst EN.
func Parse(s string) Lang {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "de") {
		return DE
	}
	return EN
}

// FromRequest liest die Sprache aus Accept-Language (erste Angabe zählt).
// Das Frontend schickt dort die in der Oberfläche gewählte Sprache.
func FromRequest(r *http.Request) Lang {
	first, _, _ := strings.Cut(r.Header.Get("Accept-Language"), ",")
	return Parse(first)
}

// Msg ist eine übersetzbare Meldung.
type Msg struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// M erzeugt eine Meldung; kv sind abwechselnd Name und Wert.
func M(code string, kv ...string) Msg {
	m := Msg{Code: code}
	if len(kv) > 1 {
		m.Params = make(map[string]string, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			m.Params[kv[i]] = kv[i+1]
		}
	}
	return m
}

// IsZero meldet eine leere Meldung.
func (m Msg) IsZero() bool { return m.Code == "" }

// Raw verpackt bereits fertigen Text (z. B. Fehlermeldungen fremder Dienste).
func Raw(text string) Msg { return M("raw", "text", text) }

// T rendert m in lang. Platzhalter {name} werden durch Parameter ersetzt.
// Parameterwerte mit Präfix "@" sind Verweise auf andere Codes (Ref), mit
// "@@" eingebettete Meldungen (Nest) – beide werden mitübersetzt. Der Code
// "join" verbindet eingebettete Meldungen (Params["items"]) mit "; ".
func T(lang Lang, m Msg) string {
	if m.IsZero() {
		return ""
	}
	if m.Code == "cf.unreachable" {
		m = cleanCFUnreachable(m)
	}
	if m.Code == "join" {
		var items []Msg
		_ = json.Unmarshal([]byte(m.Params["items"]), &items)
		return Join(lang, items)
	}
	e, ok := catalog[m.Code]
	if !ok {
		return m.Code
	}
	s := e.en
	if lang == DE {
		s = e.de
	}
	if len(m.Params) == 0 {
		return s
	}
	keys := make([]string, 0, len(m.Params))
	for k := range m.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		v := m.Params[k]
		if raw, ok := strings.CutPrefix(v, "@@"); ok {
			var nested Msg
			if json.Unmarshal([]byte(raw), &nested) == nil {
				v = T(lang, nested)
			}
		} else if code, ok := strings.CutPrefix(v, "@"); ok {
			v = T(lang, Msg{Code: code})
		}
		pairs = append(pairs, "{"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

// Join übersetzt mehrere Meldungen und verbindet sie mit "; ".
func Join(lang Lang, msgs []Msg) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if s := T(lang, m); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// Ref verweist in einem Parameter auf einen anderen Code (wird mitübersetzt).
func Ref(code string) string { return "@" + code }

// Nest bettet eine Meldung als Parameterwert ein (wird mitübersetzt).
func Nest(m Msg) string {
	b, _ := json.Marshal(m)
	return "@@" + string(b)
}

// JoinMsgs fasst mehrere Meldungen zu einer zusammen; eine einzelne bleibt,
// wie sie ist.
func JoinMsgs(msgs ...Msg) Msg {
	var items []Msg
	for _, m := range msgs {
		if !m.IsZero() {
			items = append(items, m)
		}
	}
	switch len(items) {
	case 0:
		return Msg{}
	case 1:
		return items[0]
	}
	b, _ := json.Marshal(items)
	return Msg{Code: "join", Params: map[string]string{"items": string(b)}}
}

// Equal vergleicht zwei Meldungen inhaltlich.
func Equal(a, b Msg) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// Encode liefert die Meldung als JSON (für die Datenbank); leer → "".
func Encode(m Msg) string {
	if m.IsZero() {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// Decode liest Encode-JSON; ungültig oder leer → leere Meldung.
func Decode(s string) Msg {
	var m Msg
	if s != "" {
		_ = json.Unmarshal([]byte(s), &m)
	}
	return m
}

// Error ist ein Fehler mit übersetzbarer Meldung. Error() liefert Englisch
// (für Logs).
type Error struct {
	Msg  Msg
	Wrap error
}

func (e *Error) Error() string { return T(EN, e.Msg) }
func (e *Error) Unwrap() error { return e.Wrap }

// E erzeugt einen übersetzbaren Fehler.
func E(code string, kv ...string) error { return &Error{Msg: M(code, kv...)} }

// Wrap erzeugt einen übersetzbaren Fehler mit Ursache im Parameter "detail"
// (ist die Ursache selbst übersetzbar, wird sie mitübersetzt).
func Wrap(err error, code string, kv ...string) error {
	return &Error{Msg: M(code, append(kv, "detail", Nest(FromError(err)))...), Wrap: err}
}

// Localizer ist ein Fehler, der seine Meldung selbst liefert (z. B.
// Cloudflare-API-Fehler).
type Localizer interface {
	LocalizedMsg() Msg
}

// FromError liefert die Meldung zu err; unbekannte Fehler werden als
// Rohtext durchgereicht.
func FromError(err error) Msg {
	if err == nil {
		return Msg{}
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Msg
	}
	var l Localizer
	if errors.As(err, &l) {
		return l.LocalizedMsg()
	}
	return Raw(err.Error())
}
