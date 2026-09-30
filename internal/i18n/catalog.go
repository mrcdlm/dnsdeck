package i18n

type entry struct{ de, en string }

// catalog enthält alle Texte, die der Server ausliefert. Platzhalter: {name}.
var catalog = map[string]entry{
	"raw": {"{text}", "{text}"},

	// Allgemein / API
	"api.not_found":            {"nicht gefunden", "not found"},
	"api.bad_request":          {"ungültige Anfrage", "invalid request"},
	"api.internal":             {"interner Fehler", "internal error"},
	"api.unauthorized":         {"nicht angemeldet", "not signed in"},
	"api.invalid_id":           {"ungültige ID", "invalid ID"},
	"api.invalid_param":        {"{name} ungültig", "invalid {name}"},
	"api.family_invalid":       {"family muss ipv4 oder ipv6 sein", "family must be ipv4 or ipv6"},
	"api.before_invalid":       {"before muss ein RFC3339-Zeitpunkt sein", "before must be an RFC 3339 timestamp"},
	"api.refresh_failed":       {"Aktualisierung fehlgeschlagen", "Update failed"},
	"api.live_unavailable":     {"Live-Updates nicht verfügbar", "Live updates are not available"},
	"api.settings_unavailable": {"Einstellungen nicht verfügbar", "Settings are not available"},
	"auth.wrong_password":      {"Passwort falsch", "Wrong password"},
	"auth.too_many_attempts":   {"Zu viele Fehlversuche, bitte kurz warten", "Too many failed attempts, please wait a moment"},

	// Zustände
	"state.on":        {"an", "on"},
	"state.off":       {"aus", "off"},
	"tunnel.healthy":  {"verbunden", "connected"},
	"tunnel.degraded": {"eingeschränkt", "degraded"},
	"tunnel.down":     {"getrennt", "disconnected"},
	"tunnel.inactive": {"inaktiv", "inactive"},

	// Cloudflare
	"cf.not_configured":      {"Cloudflare ist nicht konfiguriert (CF_API_TOKEN fehlt)", "Cloudflare is not configured (CF_API_TOKEN missing)"},
	"cf.zones_failed":        {"Zonen konnten nicht geladen werden: {detail}", "Could not load zones: {detail}"},
	"cf.api_http":            {"Cloudflare-API: HTTP {status}", "Cloudflare API: HTTP {status}"},
	"cf.api_error":           {"Cloudflare-API: HTTP {status}: {detail}", "Cloudflare API: HTTP {status}: {detail}"},
	"cf.unreachable":         {"Cloudflare nicht erreichbar: {detail}", "Cloudflare unreachable: {detail}"},
	"cf.multiple_records":    {"{count} {type}-Einträge für {name} vorhanden – bitte bei Cloudflare auf einen reduzieren", "{count} {type} records exist for {name} – please reduce them to one at Cloudflare"},
	"cf.tunnel_permission":   {"Kein Zugriff auf Tunnels – das Token braucht Account → Cloudflare Tunnel → Read und CF_ACCOUNT_ID muss stimmen ({detail})", "No access to tunnels – the token needs Account → Cloudflare Tunnel → Read and CF_ACCOUNT_ID must be correct ({detail})"},
	"tunnels.not_configured": {"Tunnel-Monitoring nicht konfiguriert (CF_API_TOKEN/CF_ACCOUNT_ID)", "Tunnel monitoring is not configured (CF_API_TOKEN/CF_ACCOUNT_ID)"},

	// Records
	"dnscheck.disabled":        {"DNS-Verbreitungsprüfung ist abgeschaltet (DNSCHECK_RESOLVERS=off)", "DNS propagation check is disabled (DNSCHECK_RESOLVERS=off)"},
	"dnscheck.no_ip":           {"Der Record hat noch keine IP – erst abgleichen", "The record has no IP yet – sync it first"},
	"record.not_found":         {"Record nicht gefunden", "Record not found"},
	"record.conflict":          {"{type}-Eintrag für {name} wird bereits verwaltet", "The {type} record for {name} is already managed"},
	"record.zone_unknown":      {"Zone unbekannt", "Unknown zone"},
	"record.name_outside_zone": {"Name muss in der Zone {zone} liegen", "The name must be within the zone {zone}"},
	"record.name_too_long":     {"Name zu lang", "Name too long"},
	"record.name_invalid":      {"ungültiger Name: {name}", "invalid name: {name}"},
	"record.type_invalid":      {"Typ muss A oder AAAA sein", "Type must be A or AAAA"},
	"record.ttl_invalid":       {"TTL muss 1 (automatisch) oder 60–86400 Sekunden sein", "TTL must be 1 (automatic) or 60–86400 seconds"},

	// DDNS-Abgleich
	"ddns.no_ip":           {"keine öffentliche {family}-Adresse bekannt", "no public {family} address known"},
	"ddns.created":         {"bei Cloudflare angelegt", "created at Cloudflare"},
	"ddns.adopted":         {"bestehenden Eintrag übernommen (Proxy {proxy}, TTL {ttl})", "adopted existing record (proxy {proxy}, TTL {ttl})"},
	"ddns.adopted_short":   {"bestehenden Eintrag übernommen", "adopted existing record"},
	"ddns.proxy_change":    {"Proxy: {from} → {to}", "Proxy: {from} → {to}"},
	"ddns.ttl_change":      {"TTL: {from} → {to}", "TTL: {from} → {to}"},
	"ddns.recovered":       {"Abgleich wieder erfolgreich", "Sync successful again"},
	"ddns.recovered_after": {"Abgleich wieder erfolgreich (vorher: {detail})", "Sync successful again (previously: {detail})"},

	// IP-Erkennung
	"ip.no_source":     {"keine Quelle erreichbar", "no source reachable"},
	"ip.single_source": {"nur eine Quelle meldet {ip} – Wechsel nicht bestätigt", "only one source reports {ip} – change not confirmed"},
	"ip.no_majority":   {"keine Mehrheit ({votes} von {total} für {ip})", "no majority ({votes} of {total} for {ip})"},
	"ip.save_failed":   {"Speichern fehlgeschlagen", "Saving failed"},
	"ip.http":          {"HTTP {status}", "HTTP {status}"},
	"ip.not_ip":        {"keine IP-Adresse: {value}", "not an IP address: {value}"},
	"ip.not_v4":        {"keine IPv4-Adresse: {ip}", "not an IPv4 address: {ip}"},
	"ip.not_v6":        {"keine IPv6-Adresse: {ip}", "not an IPv6 address: {ip}"},
	"ip.not_public":    {"keine öffentliche Adresse: {ip}", "not a public address: {ip}"},
	"ip.trace_no_ip":   {"keine ip=-Zeile in der Trace-Antwort", "no ip= line in trace response"},

	// Einstellungen
	"settings.ip_interval_range":     {"IP-Prüfintervall muss zwischen {min} und {max} liegen", "The IP check interval must be between {min} and {max}"},
	"settings.tunnel_interval_range": {"Tunnel-Intervall muss zwischen {min} und {max} liegen", "The tunnel interval must be between {min} and {max}"},
	"settings.source_unknown":        {"unbekannte IP-Quelle {name}", "unknown IP source {name}"},
	"settings.source_duplicate":      {"IP-Quelle {name} doppelt", "duplicate IP source {name}"},
	"settings.sources_min":           {"mindestens {min} IP-Quellen auswählen (Mehrheitsentscheid)", "select at least {min} IP sources (majority vote)"},
	"settings.language_invalid":      {"Sprache muss de oder en sein", "Language must be de or en"},

	// Webhooks
	"webhook.not_found":        {"Webhook nicht gefunden", "Webhook not found"},
	"webhook.unavailable":      {"Webhooks nicht verfügbar", "Webhooks are not available"},
	"webhook.env_prefix":       {"nur Env-Variablen mit Präfix {prefix} sind erlaubt (nicht {name})", "only environment variables with the prefix {prefix} are allowed (not {name})"},
	"webhook.env_missing":      {"Env-Variable {name} ist nicht gesetzt", "environment variable {name} is not set"},
	"webhook.in_url":           {"URL: {detail}", "URL: {detail}"},
	"webhook.in_header":        {"Header {name}: {detail}", "Header {name}: {detail}"},
	"webhook.template":         {"Template: {detail}", "Template: {detail}"},
	"webhook.template_json":    {"Template ergibt kein gültiges JSON – Texte mit {{json .Title}} einsetzen", "The template does not produce valid JSON – insert text with {{json .Title}}"},
	"webhook.name_invalid":     {"Name fehlt oder ist zu lang", "Name is missing or too long"},
	"webhook.method_invalid":   {"Methode muss POST, PUT, PATCH oder GET sein", "Method must be POST, PUT, PATCH or GET"},
	"webhook.url_missing":      {"URL fehlt", "URL is missing"},
	"webhook.url_scheme":       {"URL muss mit http:// oder https:// beginnen", "URL must start with http:// or https://"},
	"webhook.header_name":      {"ungültiger Header-Name {name}", "invalid header name {name}"},
	"webhook.header_forbidden": {"Header {name} ist nicht erlaubt", "Header {name} is not allowed"},
	"webhook.header_newline":   {"Header {name} enthält einen Zeilenumbruch", "Header {name} contains a line break"},
	"webhook.event_unknown":    {"unbekanntes Ereignis {name}", "unknown event {name}"},
	"webhook.secret_header":    {"Header {name} enthält vermutlich ein Geheimnis im Klartext – bitte als ${WEBHOOK_NAME} eintragen und den Wert in .env setzen", "Header {name} probably contains a plain-text secret – use ${WEBHOOK_NAME} and set the value in .env"},
	"webhook.secret_userinfo":  {"URL enthält Benutzer/Passwort – bitte als ${WEBHOOK_NAME} eintragen und den Wert in .env setzen", "The URL contains a user name/password – use ${WEBHOOK_NAME} and set the value in .env"},
	"webhook.secret_query":     {"URL-Parameter {name} enthält vermutlich ein Geheimnis – bitte als ${WEBHOOK_NAME} eintragen und den Wert in .env setzen", "URL parameter {name} probably contains a secret – use ${WEBHOOK_NAME} and set the value in .env"},
	"webhook.secret_url":       {"Diese URL enthält selbst das Geheimnis (Token im Pfad) – bitte als ${WEBHOOK_NAME} eintragen und den Wert in .env setzen", "This URL itself is the secret (token in the path) – use ${WEBHOOK_NAME} and set the value in .env"},
	"webhook.bad_url":          {"ungültige URL (nach Einsetzen der Env-Variablen)", "invalid URL (after inserting environment variables)"},
	"webhook.unreachable":      {"nicht erreichbar: {detail}", "unreachable: {detail}"},
	"webhook.redirect":         {"HTTP {status} – Weiterleitung auf einen anderen Host wird aus Sicherheitsgründen nicht verfolgt", "HTTP {status} – redirects to another host are not followed for security reasons"},
	"webhook.http":             {"HTTP {status}", "HTTP {status}"},

	// Benachrichtigungen
	"notify.ip_change.title":          {"Neue öffentliche {family}-Adresse", "New public {family} address"},
	"notify.ip_change.message":        {"{old} → {new}", "{old} → {new}"},
	"notify.update_failed.title":      {"DNS-Update fehlgeschlagen: {name} ({type})", "DNS update failed: {name} ({type})"},
	"notify.update_failed.message":    {"Soll-IP {ip}: {detail}", "Target IP {ip}: {detail}"},
	"notify.update_recovered.title":   {"DNS-Update wieder OK: {name} ({type})", "DNS update OK again: {name} ({type})"},
	"notify.update_recovered.message": {"{name} zeigt auf {ip}.", "{name} points to {ip}."},
	"notify.tunnel.title":             {"Tunnel {name}: {to}", "Tunnel {name}: {to}"},
	"notify.tunnel.message":           {"Status {from} → {to}", "Status {from} → {to}"},
	"notify.test.title":               {"dnsdeck: Testnachricht", "dnsdeck: test message"},
	"notify.test.message":             {"Benachrichtigungen funktionieren.", "Notifications are working."},
}
