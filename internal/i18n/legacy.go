package i18n

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Versionen vor 0.2.0 haben Meldungen nur als deutschen Text gespeichert.
// Diese Texte entsprechen den deutschen Katalogeinträgen; ParseLegacy wandelt
// sie zurück in Code und Parameter, damit sie übersetzt angezeigt werden.

type legacyPattern struct {
	code    string
	re      *regexp.Regexp
	names   []string
	literal int // Anzahl fester Zeichen – spezifischere Muster zuerst
}

var (
	legacyOnce     sync.Once
	legacyPatterns []legacyPattern
	legacyRefs     map[string]string // deutscher Text → Code (an/aus, Tunnel-Status)
)

var placeholder = regexp.MustCompile(`\{(\w+)\}`)

func buildLegacy() {
	legacyRefs = map[string]string{}
	for code, e := range catalog {
		if strings.HasPrefix(code, "state.") || strings.HasPrefix(code, "tunnel.") {
			legacyRefs[e.de] = code
		}
		if code == "raw" {
			continue
		}
		var (
			b       strings.Builder
			names   []string
			literal int
			last    int
		)
		b.WriteString("^")
		for _, loc := range placeholder.FindAllStringSubmatchIndex(e.de, -1) {
			lit := e.de[last:loc[0]]
			b.WriteString(regexp.QuoteMeta(lit))
			literal += len(lit)
			b.WriteString("(.+?)")
			names = append(names, e.de[loc[2]:loc[3]])
			last = loc[1]
		}
		lit := e.de[last:]
		b.WriteString(regexp.QuoteMeta(lit))
		literal += len(lit)
		b.WriteString("$")
		if literal == 0 {
			continue // nur Platzhalter – würde alles erkennen
		}
		legacyPatterns = append(legacyPatterns, legacyPattern{code: code,
			re: regexp.MustCompile(b.String()), names: names, literal: literal})
	}
	sort.Slice(legacyPatterns, func(i, j int) bool {
		if legacyPatterns[i].literal != legacyPatterns[j].literal {
			return legacyPatterns[i].literal > legacyPatterns[j].literal
		}
		return legacyPatterns[i].code < legacyPatterns[j].code
	})
}

// ParseLegacy erkennt einen deutschen Alttext als Katalogmeldung. ok ist nur
// true, wenn die Meldung auf Deutsch wieder genau den Text ergibt.
func ParseLegacy(text string) (Msg, bool) {
	legacyOnce.Do(buildLegacy)
	m, ok := parseLegacy(text)
	if !ok || T(DE, m) != text {
		return Msg{}, false
	}
	return m, true
}

func parseLegacy(text string) (Msg, bool) {
	if text == "" {
		return Msg{}, false
	}
	// Mehrere Änderungen, z. B. "Proxy: aus → an; TTL: 5 min → Auto"
	if strings.Contains(text, "; ") {
		var items []Msg
		for part := range strings.SplitSeq(text, "; ") {
			m, ok := parseLegacy(part)
			if !ok {
				items = nil
				break
			}
			items = append(items, m)
		}
		if len(items) > 1 {
			return JoinMsgs(items...), true
		}
	}
	for _, p := range legacyPatterns {
		sub := p.re.FindStringSubmatch(text)
		if sub == nil {
			continue
		}
		params := make(map[string]string, len(p.names))
		for i, name := range p.names {
			v := sub[i+1]
			if code, ok := legacyRefs[v]; ok {
				v = Ref(code)
			} else if name == "detail" {
				if nested, ok := parseLegacy(v); ok {
					v = Nest(nested)
				}
			}
			params[name] = v
		}
		if len(params) == 0 {
			params = nil
		}
		return Msg{Code: p.code, Params: params}, true
	}
	return Msg{}, false
}
