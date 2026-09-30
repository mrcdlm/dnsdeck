package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

func TestM4AuthRequired(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{
		{"GET", "/api/settings"}, {"PUT", "/api/settings"}, {"GET", "/api/info"}, {"GET", "/api/tunnels/history"},
		{"GET", "/api/webhooks"}, {"POST", "/api/webhooks"}, {"PUT", "/api/webhooks/1"}, {"DELETE", "/api/webhooks/1"},
		{"POST", "/api/webhooks/1/test"}, {"POST", "/api/webhooks/preview"},
	} {
		if resp := e.do(t, p.m, p.path, "", nil); resp.StatusCode != 401 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
}

func TestSettingsAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	got := decode[settingsDTO](t, e.do(t, "GET", "/api/settings", "", c))
	if got.IPCheckIntervalSeconds != 300 || got.TunnelIntervalSeconds != 60 || len(got.IPSources) != 3 ||
		!got.IPSources[0].Enabled || got.Limits["ip_sources_min"] != 2 {
		t.Fatalf("defaults: %+v", got)
	}

	body := `{"ip_check_interval_seconds":120,"tunnel_interval_seconds":30,
		"ip_sources":[{"name":"icanhazip","enabled":true},{"name":"cloudflare","enabled":true},{"name":"ipify","enabled":false}]}`
	resp := e.do(t, "PUT", "/api/settings", body, c)
	got = decode[settingsDTO](t, resp)
	if resp.StatusCode != 200 || got.IPCheckIntervalSeconds != 120 || got.IPSources[0].Name != "icanhazip" ||
		got.IPSources[2].Enabled {
		t.Fatalf("nach PUT: %d %+v", resp.StatusCode, got)
	}

	for _, bad := range []string{
		`{"ip_check_interval_seconds":5,"tunnel_interval_seconds":30,"ip_sources":[{"name":"ipify","enabled":true},{"name":"cloudflare","enabled":true}]}`,
		`{"ip_check_interval_seconds":120,"tunnel_interval_seconds":30,"ip_sources":[{"name":"ipify","enabled":true}]}`,
		`{"ip_check_interval_seconds":120,"tunnel_interval_seconds":30,"ip_sources":[{"name":"x","enabled":true},{"name":"ipify","enabled":true}]}`,
	} {
		resp := e.do(t, "PUT", "/api/settings", bad, c)
		if resp.StatusCode != 400 {
			t.Errorf("%s: %d", bad, resp.StatusCode)
		}
	}
}

func TestWebhooksAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	body := fmt.Sprintf(`{"name":"Test","method":"post","url":"%s/${WEBHOOK_PFAD}",
		"headers":[{"name":"X-Token","value":"${WEBHOOK_FEHLT}"},{"name":"","value":"leer wird entfernt"}],
		"content_type":"application/json","body_template":"{\"text\": {{json .Title}}}",
		"events":["tunnel_status","ip_change","tunnel_status"]}`, e.hookURL)
	resp := e.do(t, "POST", "/api/webhooks", body, c)
	wh := decode[webhookDTO](t, resp)
	if resp.StatusCode != 201 || wh.Method != "POST" || len(wh.Headers) != 1 ||
		strings.Join(wh.Events, ",") != "ip_change,tunnel_status" || strings.Join(wh.MissingEnv, ",") != "WEBHOOK_FEHLT" {
		t.Fatalf("create: %d %+v", resp.StatusCode, wh)
	}

	// Fehlende Variable → Test schlägt mit verständlicher Meldung fehl
	res := decode[notify.TestResult](t, e.do(t, "POST", fmt.Sprintf("/api/webhooks/%d/test", wh.ID), "", c))
	if res.OK || !strings.Contains(res.Error, "WEBHOOK_FEHLT ist nicht gesetzt") {
		t.Fatalf("test ohne Variable: %+v", res)
	}

	// Header entfernen → Test zugestellt, gespeicherte URL enthält nur den Platzhalter
	upd := strings.Replace(body, `{"name":"X-Token","value":"${WEBHOOK_FEHLT}"},`, "", 1)
	resp = e.do(t, "PUT", fmt.Sprintf("/api/webhooks/%d", wh.ID), upd, c)
	if resp.StatusCode != 200 {
		t.Fatalf("update: %d", resp.StatusCode)
	}
	res = decode[notify.TestResult](t, e.do(t, "POST", fmt.Sprintf("/api/webhooks/%d/test", wh.ID), "", c))
	if !res.OK || e.hooks.Load() != 1 {
		t.Fatalf("test: %+v hooks=%d", res, e.hooks.Load())
	}
	list := decode[[]webhookDTO](t, e.do(t, "GET", "/api/webhooks", "", c))
	if len(list) != 1 || list[0].LastSentAt == nil || strings.Contains(list[0].URL, "geheimer-pfad") {
		t.Fatalf("list: %+v", list)
	}

	// Vorschau ohne Speichern, Platzhalter bleiben sichtbar
	prev := decode[notify.Request](t, e.do(t, "POST", "/api/webhooks/preview",
		`{"method":"POST","url":"https://x.example/${WEBHOOK_PFAD}","content_type":"text/plain",
		  "body_template":"{{.Title}} / {{env \"WEBHOOK_PFAD\"}}","event_type":"ip_change"}`, c))
	if prev.URL != "https://x.example/${WEBHOOK_PFAD}" || prev.Body != "Neue öffentliche IPv4-Adresse / ${WEBHOOK_PFAD}" {
		t.Fatalf("preview: %+v", prev)
	}

	// Ungültig: fremde Variable, kaputtes Template
	for _, bad := range []string{
		`{"name":"x","url":"https://x/${CF_API_TOKEN}","events":[]}`,
		`{"name":"x","url":"https://x","content_type":"application/json","body_template":"{{.Title","events":[]}`,
	} {
		if resp := e.do(t, "POST", "/api/webhooks", bad, c); resp.StatusCode != 400 {
			t.Errorf("%s: %d", bad, resp.StatusCode)
		}
	}

	if resp := e.do(t, "DELETE", fmt.Sprintf("/api/webhooks/%d", wh.ID), "", c); resp.StatusCode != 204 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", fmt.Sprintf("/api/webhooks/%d/test", wh.ID), "", c); resp.StatusCode != 404 {
		t.Fatalf("test gelöscht: %d", resp.StatusCode)
	}

	info := decode[Info](t, e.do(t, "GET", "/api/info", "", c))
	if info.Version != "v1.2.3" || !info.CFTokenSet {
		t.Fatalf("info: %+v", info)
	}
}

func TestHistoryFilters(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	for i := range 3 {
		e.do(t, "POST", "/api/records", fmt.Sprintf(`{"zone_id":"z1","name":"h%d.example.com","type":"A"}`, i), c)
	}
	all := decode[[]store.UpdateLogEntry](t, e.do(t, "GET", "/api/updates?limit=2", "", c))
	if len(all) != 2 {
		t.Fatalf("limit: %d", len(all))
	}
	older := decode[[]store.UpdateLogEntry](t, e.do(t, "GET", fmt.Sprintf("/api/updates?before_id=%d", all[1].ID), "", c))
	if len(older) != 1 || older[0].RecordName != "h0.example.com" {
		t.Fatalf("before_id: %+v", older)
	}
	if l := decode[[]store.UpdateLogEntry](t, e.do(t, "GET", "/api/updates?result=error", "", c)); len(l) != 0 {
		t.Fatalf("result-Filter: %+v", l)
	}
	for _, bad := range []string{"/api/updates?result=kaputt", "/api/ip/history?family=ipv5", "/api/tunnels/history?before=gestern"} {
		if resp := e.do(t, "GET", bad, "", c); resp.StatusCode != 400 {
			t.Errorf("%s: %d", bad, resp.StatusCode)
		}
	}

	e.do(t, "POST", "/api/tunnels/refresh", "", c)
	ch := decode[[]store.TunnelChange](t, e.do(t, "GET", "/api/tunnels/history", "", c))
	if len(ch) != 1 || ch[0].TunnelName != "home" || ch[0].From != "" || ch[0].To != "healthy" {
		t.Fatalf("tunnel history: %+v", ch)
	}
}
