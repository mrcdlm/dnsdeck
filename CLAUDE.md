# CLAUDE.md – Projekt „dnsdeck“ (Arbeitstitel)

Selbst gehosteter DDNS-Updater mit modernem Web-Dashboard und Cloudflare-Tunnel-Monitoring.
Läuft als **ein einziger Docker-Container** auf jedem Linux-Host mit Docker Compose (Referenzsystem: openSUSE MicroOS).

## Ziele
1. DNS-Einträge automatisch auf die aktuelle öffentliche IP aktualisieren (IPv4 + IPv6).
2. Aktuelle öffentliche IP gut sichtbar anzeigen (inkl. Verlauf von IP-Wechseln).
3. Status aller Cloudflare Tunnels überwachen (healthy / degraded / down / inactive, aktive Verbindungen, Colos).
4. Modernes, schnelles Web-UI (Dark Mode, responsive, Live-Updates).

## Tech-Stack (verbindlich)
- **Backend:** Go (aktuelle stabile Version), Standard-Library `net/http` + `chi` Router
- **Datenbank:** SQLite (`modernc.org/sqlite`, CGO-frei) – Datei unter `/data/app.db`
- **Frontend:** React + TypeScript + Vite, Tailwind CSS, shadcn/ui, TanStack Query, lucide-react Icons, Recharts für Diagramme
- **Auslieferung:** Frontend-Build wird per `go:embed` ins Go-Binary eingebettet → ein Binary, ein Container
- **Live-Updates:** Server-Sent Events (`/api/events`)
- **Container:** Multi-Stage-Build, finales Image `gcr.io/distroless/static` oder `scratch`, läuft als non-root, Ziel < 30 MB
- **CI:** GitHub Actions → Multi-Arch-Image (amd64 + arm64) nach `ghcr.io`

## Verzeichnisstruktur
```
/cmd/server          main.go
/internal/config     Env-Variablen + Settings aus DB
/internal/ipdetect   Ermittlung öffentlicher IPv4/IPv6
/internal/providers  DNS-Provider (Interface + cloudflare/ als erste Implementierung)
/internal/tunnels    Cloudflare-Tunnel-Monitoring
/internal/notify     Benachrichtigungen (generische Webhooks)
/internal/scheduler  periodische Jobs
/internal/store      SQLite, Migrationen
/internal/api        HTTP-Handler, SSE
/web                 React-Frontend (Vite)
/deploy              docker-compose.yml, Beispiel-.env
```

## Funktionen im Detail

### Öffentliche IP
- Mehrere Quellen, Fallback-Reihenfolge konfigurierbar:
  `https://1.1.1.1/cdn-cgi/trace`, `https://api.ipify.org`, `https://api6.ipify.org`, `https://icanhazip.com`
- Mehrheitsentscheid / Plausibilitätscheck, damit eine falsche Quelle keinen Update auslöst
- IP-Wechsel werden mit Zeitstempel in der DB protokolliert

### DDNS
- Provider-Interface (`Provider` mit `GetRecord`, `UpdateRecord`), damit später weitere Anbieter ergänzt werden können
- Cloudflare zuerst: A- und AAAA-Records, `proxied`-Flag und TTL pro Record einstellbar
- Update nur, wenn sich die IP tatsächlich unterscheidet (vorher Ist-Zustand bei Cloudflare abfragen)
- Intervall konfigurierbar (Default 5 min), manueller „Jetzt aktualisieren“-Button
- Protokoll jedes Update-Versuchs (Erfolg/Fehler, alte/neue IP)

### Cloudflare-Tunnel-Monitoring
- API: `GET /accounts/{account_id}/cfd_tunnel?is_deleted=false`
- Anzeigen: Name, Status, Anzahl Verbindungen, Colo-Namen, Client-Version, `conns_active_at` / `conns_inactive_at`
- Polling-Intervall konfigurierbar (Default 60 s), Statusänderungen in DB speichern → Uptime-Verlauf
- Optional später: zugehörige Public Hostnames aus der Tunnel-Konfiguration anzeigen

### Benachrichtigungen (Meilenstein 4)
- Bei IP-Wechsel, fehlgeschlagenem Update, wieder erfolgreichem Update, Tunnel-Statuswechsel
- **Nur generische Webhooks**, keine produktspezifischen Kanäle: beliebig viele, je Webhook
  Methode, URL, Header, Content-Type, Body-Template (Go `text/template`) und eigene Ereignisauswahl
- Konfiguration in der UI (Einstellungen), gespeichert in der DB; Vorschau mit Beispielereignis und Test je Webhook
- Dienste wie ntfy, Gotify, Discord, Slack, Telegram, Home Assistant nur als **Vorlagen** im Frontend,
  kein eigener Code pro Dienst

### UI-Seiten
- **Dashboard:** große IP-Karte (v4/v6, seit wann), Status-Kacheln für Records und Tunnels, letzte Ereignisse
- **Records:** Tabelle aller verwalteten Records, hinzufügen/bearbeiten/löschen, Status-Badge
- **Tunnels:** Karten je Tunnel mit Status, Verbindungen, Uptime-Balken der letzten 24 h / 7 Tage
- **Verlauf:** IP-Wechsel und Update-Log, filterbar
- **Einstellungen:** Intervalle, IP-Quellen, Webhooks

## Konfiguration & Sicherheit
- Secrets **nur** per Env-Variable, niemals in DB, Logs oder API-Antworten:
  `CF_API_TOKEN`, `CF_ACCOUNT_ID`, `APP_PASSWORD`
- Webhook-Geheimnisse (Tokens, geheime URLs) ebenfalls nur per Env-Variable mit Präfix `WEBHOOK_`;
  in der DB stehen nur Platzhalter: `${WEBHOOK_NAME}` in URL/Headern, `{{env "WEBHOOK_NAME"}}` im Body
  - Nur Variablen mit Präfix `WEBHOOK_` sind auflösbar (kein Zugriff auf `CF_API_TOKEN`, `APP_PASSWORD` o. ä.)
  - Platzhalter nur im konfigurierten Text ersetzen, nie in Ereignisdaten; Vorschau und Fehlermeldungen ohne aufgelöste Werte
- Benötigte Token-Rechte: Zone → DNS → Edit, Zone → Zone → Read, Account → Cloudflare Tunnel → Read
- Login mit einem Passwort (bcrypt-Vergleich gegen `APP_PASSWORD`), Session-Cookie `HttpOnly`, `SameSite=Strict`
- Healthcheck-Endpoint `/healthz` (ohne Auth)
- Port Default `8080`, Daten unter `/data`

## Arbeitsweise für Claude
- In kleinen, lauffähigen Schritten arbeiten; nach jedem Meilenstein committen
- Go: `go vet` und Tests (`go test ./...`) müssen grün sein; Cloudflare-API in Tests mocken
- Frontend: `npm run build` und `npm run lint` müssen durchlaufen
- Keine Secrets in Code, Tests oder Beispieldateien – `.env.example` mit Platzhaltern verwenden
- Bei Unklarheiten zuerst fragen, bevor Architektur geändert wird

## Meilensteine
1. **Grundgerüst:** Go-Server, SQLite + Migrationen, IP-Erkennung, Dashboard mit IP-Anzeige, Dockerfile, Compose
2. **DDNS Cloudflare:** Records verwalten, Scheduler, Update-Log, Records-Seite
3. **Tunnel-Monitoring:** Tunnels-Seite, Statusverlauf, SSE-Live-Updates
4. **Benachrichtigungen (generische Webhooks) + Verlauf-Seite + Einstellungen**
5. **CI/CD:** GitHub Actions, Multi-Arch-Image auf ghcr.io, Release-Tags
