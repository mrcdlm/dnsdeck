package notify

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

// EnvPrefix: nur Env-Variablen mit diesem Präfix dürfen in Webhooks stehen.
const EnvPrefix = "WEBHOOK_"

// Methods, die ein Webhook verwenden darf.
var Methods = []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodGet}

// Env liefert den Wert einer Env-Variablen (wie os.LookupEnv).
type Env func(name string) (string, bool)

var (
	placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	// Header/Parameter, deren Wert fast immer ein Geheimnis ist.
	secretNameRe = regexp.MustCompile(`(?i)(authorization|token|secret|passw|api[-_]?key|access[-_]?key|[-_]key$|^key$|signature|^sig$|auth)`)
	envCallRe    = regexp.MustCompile(`\benv\s+"([^"]*)"`)
	headerNameRe = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]+$")
)

// ValidationError sind Konfigurationsfehler, die dem Benutzer angezeigt werden.
type ValidationError struct{ Msgs []i18n.Msg }

func (e *ValidationError) Error() string { return i18n.Join(i18n.EN, e.Msgs) }

func checkEnvName(name string) error {
	if !strings.HasPrefix(name, EnvPrefix) || len(name) == len(EnvPrefix) {
		return i18n.E("webhook.env_prefix", "prefix", EnvPrefix, "name", strconv.Quote(name))
	}
	return nil
}

// expand ersetzt ${WEBHOOK_…} in s. Nur für Text aus der Konfiguration –
// nie für Ereignisdaten, sonst ließen sich Geheimnisse einschleusen.
func expand(s string, env Env) (string, error) {
	var msgs []i18n.Msg
	out := placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		if err := checkEnvName(name); err != nil {
			msgs = append(msgs, i18n.FromError(err))
			return ""
		}
		v, ok := env(name)
		if !ok {
			msgs = append(msgs, i18n.M("webhook.env_missing", "name", name))
		}
		return v
	})
	if len(msgs) > 0 {
		return out, &i18n.Error{Msg: i18n.JoinMsgs(msgs...)}
	}
	return out, nil
}

// ReferencedEnv liefert alle Env-Variablen, die ein Webhook verwendet.
func ReferencedEnv(w store.Webhook) []string {
	var names []string
	add := func(n string) {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	for _, s := range append([]string{w.URL}, headerValues(w)...) {
		for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
			add(m[1])
		}
	}
	for _, m := range envCallRe.FindAllStringSubmatch(w.BodyTemplate, -1) {
		add(m[1])
	}
	return names
}

// MissingEnv liefert verwendete Env-Variablen, die nicht gesetzt sind.
func MissingEnv(w store.Webhook, env Env) []string {
	var out []string
	for _, n := range ReferencedEnv(w) {
		if _, ok := env(n); !ok && checkEnvName(n) == nil {
			out = append(out, n)
		}
	}
	return out
}

func headerValues(w store.Webhook) []string {
	out := make([]string, len(w.Headers))
	for i, h := range w.Headers {
		out[i] = h.Value
	}
	return out
}

// templateData ist, was im Body-Template zur Verfügung steht.
type templateData struct {
	Type     string
	Title    string
	Message  string
	Priority int
	Time     time.Time
	Data     map[string]string
}

func funcs(env Env) template.FuncMap {
	return template.FuncMap{
		// json kodiert einen Wert als JSON (inkl. Anführungszeichen bei Strings).
		"json": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
		"env": func(name string) (string, error) {
			if err := checkEnvName(name); err != nil {
				return "", err
			}
			v, ok := env(name)
			if !ok {
				return "", i18n.E("webhook.env_missing", "name", name)
			}
			return v, nil
		},
		"mul":      func(a, b int) int { return a * b },
		"upper":    strings.ToUpper,
		"lower":    strings.ToLower,
		"urlquery": url.QueryEscape,
	}
}

// Request ist eine fertig gerenderte Webhook-Anfrage.
type Request struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

// Render baut die Anfrage für ev. Mit env == nil wird eine Vorschau ohne
// Geheimnisse erzeugt (Platzhalter bleiben sichtbar).
func Render(w store.Webhook, ev Event, env Env) (Request, error) {
	preview := env == nil
	if preview {
		env = func(name string) (string, bool) { return "${" + name + "}", true }
	}

	req := Request{Method: w.Method, Headers: http.Header{}}
	var err error
	if preview {
		_, err = expand(w.URL, env) // nur Namen prüfen
		req.URL = w.URL
	} else {
		req.URL, err = expand(w.URL, env)
	}
	if err != nil {
		return Request{}, i18n.Wrap(err, "webhook.in_url")
	}

	for _, h := range w.Headers {
		v := h.Value
		if !preview {
			if v, err = expand(h.Value, env); err != nil {
				return Request{}, i18n.Wrap(err, "webhook.in_header", "name", h.Name)
			}
		} else if _, err = expand(h.Value, env); err != nil {
			return Request{}, i18n.Wrap(err, "webhook.in_header", "name", h.Name)
		}
		req.Headers.Add(h.Name, v)
	}

	if w.Method != http.MethodGet {
		// In der Vorschau steht statt env-Werten "${NAME}" im Body; ungequotet
		// ergibt das kein JSON, obwohl es beim Senden gültig wäre. Validate
		// prüft JSON deshalb separat mit neutralem Testwert.
		body, err := renderBody(w, ev, env, !preview)
		if err != nil {
			return Request{}, err
		}
		req.Body = body
		if w.ContentType != "" && req.Headers.Get("Content-Type") == "" {
			req.Headers.Set("Content-Type", w.ContentType)
		}
	}
	return req, nil
}

func renderBody(w store.Webhook, ev Event, env Env, checkJSON bool) (string, error) {
	if strings.TrimSpace(w.BodyTemplate) == "" {
		b, _ := json.Marshal(ev)
		return string(b), nil
	}
	tpl, err := template.New("body").Funcs(funcs(env)).Option("missingkey=zero").Parse(w.BodyTemplate)
	if err != nil {
		return "", i18n.Wrap(err, "webhook.template")
	}
	var buf bytes.Buffer
	data := templateData{Type: ev.Type, Title: ev.Title, Message: ev.Message, Priority: ev.Priority,
		Time: ev.Time, Data: ev.Data}
	if err := tpl.Execute(&buf, data); err != nil {
		return "", i18n.Wrap(err, "webhook.template")
	}
	if checkJSON && strings.Contains(strings.ToLower(w.ContentType), "json") && !json.Valid(buf.Bytes()) {
		return "", i18n.E("webhook.template_json")
	}
	return buf.String(), nil
}

// Validate prüft die Konfiguration und rendert sie probeweise für alle
// Ereignistypen (ohne Geheimnisse).
func Validate(w store.Webhook) error {
	var msgs []i18n.Msg
	add := func(code string, kv ...string) { msgs = append(msgs, i18n.M(code, kv...)) }
	if strings.TrimSpace(w.Name) == "" || len(w.Name) > 100 {
		add("webhook.name_invalid")
	}
	if !slices.Contains(Methods, w.Method) {
		add("webhook.method_invalid")
	}
	// URL: Platzhalter durch Dummy ersetzen und prüfen; eine URL, die nur aus
	// einem Platzhalter besteht (z. B. Discord), wird erst beim Senden geprüft.
	if strings.TrimSpace(w.URL) == "" {
		add("webhook.url_missing")
	} else if !placeholderRe.MatchString(strings.TrimSpace(w.URL)) || placeholderRe.ReplaceAllString(w.URL, "") != "" {
		dummy := placeholderRe.ReplaceAllString(w.URL, "x")
		if u, err := url.Parse(dummy); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			add("webhook.url_scheme")
		}
	}
	for _, h := range w.Headers {
		switch {
		case !headerNameRe.MatchString(h.Name):
			add("webhook.header_name", "name", strconv.Quote(h.Name))
		case strings.EqualFold(h.Name, "Host") || strings.EqualFold(h.Name, "Content-Length"):
			add("webhook.header_forbidden", "name", h.Name)
		case strings.ContainsAny(h.Value, "\r\n"):
			add("webhook.header_newline", "name", h.Name)
		}
	}
	for _, e := range w.Events {
		if !slices.Contains(EventTypes, e) {
			add("webhook.event_unknown", "name", strconv.Quote(e))
		}
	}
	for _, n := range ReferencedEnv(w) {
		if err := checkEnvName(n); err != nil {
			msgs = append(msgs, i18n.FromError(err))
		}
	}
	msgs = append(msgs, plainSecrets(w)...)
	if len(msgs) == 0 {
		// Probe-Rendering; env-Werte durch "0" ersetzen – das ist sowohl in
		// Anführungszeichen als auch ungequotet (Zahl) gültiges JSON.
		zero := func(string) (string, bool) { return "0", true }
		for _, t := range append(slices.Clone(EventTypes), EventTest) {
			ev := Localize(SampleEvent(t), i18n.EN)
			if _, err := Render(w, ev, nil); err != nil {
				msgs = append(msgs, i18n.FromError(err))
				break
			}
			if w.Method != http.MethodGet {
				if _, err := renderBody(w, ev, zero, true); err != nil {
					msgs = append(msgs, i18n.FromError(err))
					break
				}
			}
		}
	}
	if len(msgs) > 0 {
		return &ValidationError{Msgs: msgs}
	}
	return nil
}

// Adressen, deren Pfad selbst das Geheimnis ist.
var secretURLs = []struct{ host, pathPrefix string }{
	{"discord.com", "/api/webhooks/"}, {"discordapp.com", "/api/webhooks/"},
	{"hooks.slack.com", "/"}, {"api.telegram.org", "/bot"},
}

// plainSecrets erkennt typische, direkt eingetragene Geheimnisse (statt
// ${WEBHOOK_…}), damit sie nicht in der Datenbank landen.
func plainSecrets(w store.Webhook) []i18n.Msg {
	var msgs []i18n.Msg
	for _, h := range w.Headers {
		if secretNameRe.MatchString(h.Name) && !placeholderRe.MatchString(h.Value) && strings.TrimSpace(h.Value) != "" {
			msgs = append(msgs, i18n.M("webhook.secret_header", "name", h.Name))
		}
	}
	u, err := url.Parse(w.URL)
	if err != nil {
		return msgs
	}
	if u.User != nil {
		msgs = append(msgs, i18n.M("webhook.secret_userinfo"))
	}
	for k, vals := range u.Query() {
		for _, v := range vals {
			if secretNameRe.MatchString(k) && v != "" && !placeholderRe.MatchString(v) {
				msgs = append(msgs, i18n.M("webhook.secret_query", "name", k))
			}
		}
	}
	host := strings.ToLower(u.Hostname())
	for _, s := range secretURLs {
		if (host == s.host || strings.HasSuffix(host, "."+s.host)) && strings.HasPrefix(u.Path, s.pathPrefix) &&
			!placeholderRe.MatchString(w.URL) {
			msgs = append(msgs, i18n.M("webhook.secret_url"))
		}
	}
	return msgs
}

// SampleEvent liefert ein Beispielereignis (Vorschau, Validierung, Test);
// Titel und Text sind übersetzbar (Localize).
func SampleEvent(eventType string) Event {
	t := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	switch eventType {
	case EventIPChange:
		return Event{Type: eventType, Priority: PriorityDefault, Time: t,
			TitleMsg:   i18n.M("notify.ip_change.title", "family", "IPv4"),
			MessageMsg: i18n.M("notify.ip_change.message", "old", "203.0.113.1", "new", "203.0.113.2"),
			Data:       map[string]string{"family": "ipv4", "old": "203.0.113.1", "new": "203.0.113.2"}}
	case EventUpdateFailed:
		return Event{Type: eventType, Priority: PriorityHigh, Time: t,
			TitleMsg: i18n.M("notify.update_failed.title", "name", "home.example.com", "type", "A"),
			MessageMsg: i18n.M("notify.update_failed.message", "ip", "203.0.113.2",
				"detail", i18n.Nest(i18n.M("cf.api_http", "status", "500"))),
			Data: map[string]string{"record": "home.example.com", "type": "A", "ip": "203.0.113.2", "error": "Cloudflare API: HTTP 500"}}
	case EventUpdateRecovered:
		return Event{Type: eventType, Priority: PriorityDefault, Time: t,
			TitleMsg:   i18n.M("notify.update_recovered.title", "name", "home.example.com", "type", "A"),
			MessageMsg: i18n.M("notify.update_recovered.message", "name", "home.example.com", "ip", "203.0.113.2"),
			Data:       map[string]string{"record": "home.example.com", "type": "A", "ip": "203.0.113.2"}}
	case EventTunnelStatus:
		return Event{Type: eventType, Priority: PriorityHigh, Time: t,
			TitleMsg:   i18n.M("notify.tunnel.title", "name", "home", "to", i18n.Ref("tunnel.down")),
			MessageMsg: i18n.M("notify.tunnel.message", "from", i18n.Ref("tunnel.healthy"), "to", i18n.Ref("tunnel.down")),
			Data:       map[string]string{"tunnel": "home", "tunnel_id": "…", "from": "healthy", "to": "down"}}
	case EventSiteDown:
		return Event{Type: eventType, Priority: PriorityHigh, Time: t,
			TitleMsg:   i18n.M("notify.site_down.title", "host", "nas.example.com"),
			MessageMsg: i18n.M("probe.timeout"),
			Data: map[string]string{"url": "https://nas.example.com/", "host": "nas.example.com", "status": "down",
				"error": "Timed out – no response"}}
	case EventSiteRecovered:
		return Event{Type: eventType, Priority: PriorityDefault, Time: t,
			TitleMsg:   i18n.M("notify.site_recovered.title", "host", "nas.example.com"),
			MessageMsg: i18n.M("notify.site_recovered.message", "url", "https://nas.example.com/"),
			Data: map[string]string{"url": "https://nas.example.com/", "host": "nas.example.com", "status": "up",
				"http_status": "200"}}
	case EventCertExpiring:
		return Event{Type: eventType, Priority: PriorityDefault, Time: t,
			TitleMsg:   i18n.M("notify.cert_expiring.title", "host", "nas.example.com"),
			MessageMsg: i18n.M("probe.cert_expires", "date", "2026-01-11", "days", "10"),
			Data: map[string]string{"url": "https://nas.example.com/", "host": "nas.example.com",
				"not_after": "2026-01-11T12:00:00Z", "days": "10", "issuer": "Let's Encrypt (R11)"}}
	case EventBlocklisted:
		return Event{Type: eventType, Priority: PriorityHigh, Time: t,
			TitleMsg:   i18n.M("notify.blocklist.title"),
			MessageMsg: i18n.M("notify.blocklist.message", "ip", "203.0.113.2", "lists", "SpamCop"),
			Data:       map[string]string{"ip": "203.0.113.2", "lists": "SpamCop", "zones": "bl.spamcop.net"}}
	}
	return Event{Type: EventTest, Priority: PriorityDefault, Time: time.Now().UTC(),
		TitleMsg: i18n.M("notify.test.title"), MessageMsg: i18n.M("notify.test.message"), Data: map[string]string{}}
}
