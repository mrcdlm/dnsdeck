package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"

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
	envCallRe     = regexp.MustCompile(`\benv\s+"([^"]*)"`)
	headerNameRe  = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]+$")
)

// ValidationError ist ein Konfigurationsfehler, der dem Benutzer angezeigt wird.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func checkEnvName(name string) error {
	if !strings.HasPrefix(name, EnvPrefix) || len(name) == len(EnvPrefix) {
		return fmt.Errorf("nur Env-Variablen mit Präfix %s sind erlaubt (nicht %q)", EnvPrefix, name)
	}
	return nil
}

// expand ersetzt ${WEBHOOK_…} in s. Nur für Text aus der Konfiguration –
// nie für Ereignisdaten, sonst ließen sich Geheimnisse einschleusen.
func expand(s string, env Env) (string, error) {
	var errs []error
	out := placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		if err := checkEnvName(name); err != nil {
			errs = append(errs, err)
			return ""
		}
		v, ok := env(name)
		if !ok {
			errs = append(errs, fmt.Errorf("Env-Variable %s ist nicht gesetzt", name))
		}
		return v
	})
	return out, errors.Join(errs...)
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
				return "", fmt.Errorf("Env-Variable %s ist nicht gesetzt", name)
			}
			return v, nil
		},
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
		return Request{}, fmt.Errorf("URL: %w", err)
	}

	for _, h := range w.Headers {
		v := h.Value
		if !preview {
			if v, err = expand(h.Value, env); err != nil {
				return Request{}, fmt.Errorf("Header %s: %w", h.Name, err)
			}
		} else if _, err = expand(h.Value, env); err != nil {
			return Request{}, fmt.Errorf("Header %s: %w", h.Name, err)
		}
		req.Headers.Add(h.Name, v)
	}

	if w.Method != http.MethodGet {
		body, err := renderBody(w, ev, env)
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

func renderBody(w store.Webhook, ev Event, env Env) (string, error) {
	if strings.TrimSpace(w.BodyTemplate) == "" {
		b, _ := json.Marshal(ev)
		return string(b), nil
	}
	tpl, err := template.New("body").Funcs(funcs(env)).Option("missingkey=zero").Parse(w.BodyTemplate)
	if err != nil {
		return "", fmt.Errorf("Template: %w", err)
	}
	var buf bytes.Buffer
	data := templateData{Type: ev.Type, Title: ev.Title, Message: ev.Message, Priority: ev.Priority,
		Time: ev.Time, Data: ev.Data}
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("Template: %w", err)
	}
	if strings.Contains(strings.ToLower(w.ContentType), "json") && !json.Valid(buf.Bytes()) {
		return "", errors.New("Template ergibt kein gültiges JSON – Texte mit {{json .Title}} einsetzen")
	}
	return buf.String(), nil
}

// Validate prüft die Konfiguration und rendert sie probeweise für alle
// Ereignistypen (ohne Geheimnisse).
func Validate(w store.Webhook) error {
	var msgs []string
	if strings.TrimSpace(w.Name) == "" || len(w.Name) > 100 {
		msgs = append(msgs, "Name fehlt oder ist zu lang")
	}
	if !slices.Contains(Methods, w.Method) {
		msgs = append(msgs, "Methode muss POST, PUT, PATCH oder GET sein")
	}
	// URL: Platzhalter durch Dummy ersetzen und prüfen; eine URL, die nur aus
	// einem Platzhalter besteht (z. B. Discord), wird erst beim Senden geprüft.
	if strings.TrimSpace(w.URL) == "" {
		msgs = append(msgs, "URL fehlt")
	} else if !placeholderRe.MatchString(strings.TrimSpace(w.URL)) || placeholderRe.ReplaceAllString(w.URL, "") != "" {
		dummy := placeholderRe.ReplaceAllString(w.URL, "x")
		if u, err := url.Parse(dummy); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			msgs = append(msgs, "URL muss mit http:// oder https:// beginnen")
		}
	}
	for _, h := range w.Headers {
		switch {
		case !headerNameRe.MatchString(h.Name):
			msgs = append(msgs, fmt.Sprintf("ungültiger Header-Name %q", h.Name))
		case strings.EqualFold(h.Name, "Host") || strings.EqualFold(h.Name, "Content-Length"):
			msgs = append(msgs, fmt.Sprintf("Header %s ist nicht erlaubt", h.Name))
		case strings.ContainsAny(h.Value, "\r\n"):
			msgs = append(msgs, fmt.Sprintf("Header %s enthält einen Zeilenumbruch", h.Name))
		}
	}
	for _, e := range w.Events {
		if !slices.Contains(EventTypes, e) {
			msgs = append(msgs, fmt.Sprintf("unbekanntes Ereignis %q", e))
		}
	}
	for _, n := range ReferencedEnv(w) {
		if err := checkEnvName(n); err != nil {
			msgs = append(msgs, err.Error())
		}
	}
	if len(msgs) == 0 {
		for _, t := range append(slices.Clone(EventTypes), EventTest) {
			if _, err := Render(w, SampleEvent(t), nil); err != nil {
				msgs = append(msgs, err.Error())
				break
			}
		}
	}
	if len(msgs) > 0 {
		return &ValidationError{Msg: strings.Join(msgs, "; ")}
	}
	return nil
}

// SampleEvent liefert ein Beispielereignis (Vorschau, Validierung, Test).
func SampleEvent(eventType string) Event {
	t := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	switch eventType {
	case EventIPChange:
		return Event{Type: eventType, Title: "Neue öffentliche IPv4-Adresse", Message: "203.0.113.1 → 203.0.113.2",
			Priority: PriorityDefault, Time: t, Data: map[string]string{"family": "ipv4", "old": "203.0.113.1", "new": "203.0.113.2"}}
	case EventUpdateFailed:
		return Event{Type: eventType, Title: "DNS-Update fehlgeschlagen: home.example.com (A)",
			Message: "Soll-IP 203.0.113.2: Cloudflare-API: HTTP 500", Priority: PriorityHigh, Time: t,
			Data: map[string]string{"record": "home.example.com", "type": "A", "ip": "203.0.113.2", "error": "Cloudflare-API: HTTP 500"}}
	case EventUpdateRecovered:
		return Event{Type: eventType, Title: "DNS-Update wieder OK: home.example.com (A)",
			Message: "home.example.com zeigt auf 203.0.113.2.", Priority: PriorityDefault, Time: t,
			Data: map[string]string{"record": "home.example.com", "type": "A", "ip": "203.0.113.2"}}
	case EventTunnelStatus:
		return Event{Type: eventType, Title: "Tunnel home: getrennt", Message: "Status verbunden → getrennt",
			Priority: PriorityHigh, Time: t, Data: map[string]string{"tunnel": "home", "tunnel_id": "…", "from": "healthy", "to": "down"}}
	}
	return Event{Type: EventTest, Title: "dnsdeck: Testnachricht", Message: "Benachrichtigungen funktionieren.",
		Priority: PriorityDefault, Time: time.Now().UTC(), Data: map[string]string{}}
}
