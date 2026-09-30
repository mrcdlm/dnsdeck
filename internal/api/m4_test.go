package api

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

func TestM4AuthRequired(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{
		{"GET", "/api/settings"}, {"PUT", "/api/settings"}, {"GET", "/api/notifications"},
		{"POST", "/api/notifications/test"}, {"GET", "/api/info"}, {"GET", "/api/tunnels/history"},
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
		!got.IPSources[0].Enabled || !got.NotifyEvents["ip_change"] || got.Limits["ip_sources_min"] != 2 {
		t.Fatalf("defaults: %+v", got)
	}

	body := `{"ip_check_interval_seconds":120,"tunnel_interval_seconds":30,
		"ip_sources":[{"name":"icanhazip","enabled":true},{"name":"cloudflare","enabled":true},{"name":"ipify","enabled":false}],
		"notify_events":{"ip_change":false,"tunnel_status":true}}`
	resp := e.do(t, "PUT", "/api/settings", body, c)
	got = decode[settingsDTO](t, resp)
	if resp.StatusCode != 200 || got.IPCheckIntervalSeconds != 120 || got.IPSources[0].Name != "icanhazip" ||
		got.IPSources[2].Enabled || got.NotifyEvents["ip_change"] || !got.NotifyEvents["update_failed"] {
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

func TestNotificationsAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	resp := e.do(t, "GET", "/api/notifications", "", c)
	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "geheimer-pfad") {
		t.Fatalf("URL-Pfad in API-Antwort: %s", raw)
	}
	res := decode[[]notify.TestResult](t, e.do(t, "POST", "/api/notifications/test", "", c))
	if len(res) != 1 || !res[0].OK || e.hooks.Load() != 1 {
		t.Fatalf("test: %+v hooks=%d", res, e.hooks.Load())
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
