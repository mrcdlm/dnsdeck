package probe

import (
	"strconv"
	"strings"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

// Expected sind erwartete HTTP-Statuscodes als Bereiche.
type Expected []codeRange

type codeRange struct{ from, to int }

// ParseExpected liest eine Angabe wie "200", "200,204", "2xx" oder
// "200-299,301" und liefert sie normalisiert zurück. Leer = keine Erwartung
// (jede Antwort unter 500 gilt als erreichbar).
func ParseExpected(spec string) (Expected, string, error) {
	var (
		out   Expected
		parts []string
	)
	for part := range strings.SplitSeq(spec, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		r, ok := parseCodeRange(part)
		if !ok {
			return nil, "", i18n.E("probe.expected_invalid", "value", part)
		}
		out = append(out, r)
		parts = append(parts, part)
	}
	if len(parts) > 20 {
		return nil, "", i18n.E("probe.expected_invalid", "value", spec)
	}
	return out, strings.Join(parts, ","), nil
}

func parseCodeRange(s string) (codeRange, bool) {
	valid := func(n int) bool { return n >= 100 && n <= 599 }
	// "2xx" = 200–299
	if len(s) == 3 && s[1:] == "xx" && s[0] >= '1' && s[0] <= '5' {
		base := int(s[0]-'0') * 100
		return codeRange{base, base + 99}, true
	}
	if a, b, ok := strings.Cut(s, "-"); ok {
		from, err1 := strconv.Atoi(strings.TrimSpace(a))
		to, err2 := strconv.Atoi(strings.TrimSpace(b))
		if err1 != nil || err2 != nil || !valid(from) || !valid(to) || from > to {
			return codeRange{}, false
		}
		return codeRange{from, to}, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || !valid(n) {
		return codeRange{}, false
	}
	return codeRange{n, n}, true
}

// Match meldet, ob code erwartet ist.
func (e Expected) Match(code int) bool {
	for _, r := range e {
		if code >= r.from && code <= r.to {
			return true
		}
	}
	return false
}
