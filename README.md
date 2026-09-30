# dnsdeck

Selbst gehosteter DDNS-Updater mit Web-Dashboard und Cloudflare-Tunnel-Monitoring –
ein Go-Binary mit eingebettetem React-Frontend, ausgeliefert als ein Container.

**Stand:** Meilenstein 5 (alle Meilensteine umgesetzt)
- Erkennung der öffentlichen IPv4/IPv6 per Mehrheitsentscheid mehrerer Quellen, IP-Verlauf
- DDNS mit Cloudflare: A-/AAAA-Records verwalten (Proxy, TTL), automatischer Abgleich
  (nur bei Abweichung vom Ist-Zustand), Update-Protokoll
- Tunnel-Monitoring: Status, Verbindungen, Colos, Uptime-Balken 24 h / 7 Tage
- Live-Updates per Server-Sent Events (`/api/events`)
- Benachrichtigungen über frei konfigurierbare Webhooks (Methode, URL, Header,
  Body-Template) – Vorlagen für ntfy, Gotify, Discord, Slack, Telegram, Home Assistant
- Verlauf (filterbar) und Einstellungen (Intervalle, IP-Quellen, Webhooks –
  wirken ohne Neustart)
- Dashboard, Records- und Tunnels-Seite, Login
- CI/CD: GitHub Actions, Multi-Arch-Image (amd64 + arm64) auf ghcr.io, Release-Tags

## Betrieb mit Docker Compose (Server)

```sh
# auf dem Server, z. B. /opt/stacks/dnsdeck/
# docker-compose.yml und .env.example aus deploy/ dorthin kopieren
cp .env.example .env              # Werte eintragen
mkdir -p data && sudo chown 65532:65532 data   # Container läuft als UID 65532
docker compose pull && docker compose up -d
```

Aktualisieren: `docker compose pull && docker compose up -d`. Das Dashboard ist
unter `http://<server>:8080` erreichbar (Healthcheck: `/healthz`). Nicht ohne HTTPS
ins Internet stellen – z. B. über einen Cloudflare Tunnel veröffentlichen.

Ist das Paket auf ghcr.io privat, den Server einmalig anmelden
(`docker login ghcr.io`, Token mit `read:packages`) oder das Paket auf GitHub
auf „public“ stellen – das Image enthält keine Geheimnisse.

Benötigte Cloudflare-Token-Rechte: Zone → DNS → Edit, Zone → Zone → Read,
Account → Cloudflare Tunnel → Read.

| Variable        | Pflicht | Beschreibung                                            |
|-----------------|---------|---------------------------------------------------------|
| `APP_PASSWORD`  | ja      | Dashboard-Passwort (Klartext oder bcrypt-Hash)          |
| `CF_API_TOKEN`  | für DDNS | Cloudflare-API-Token (Zone → DNS → Edit, Zone → Zone → Read) |
| `CF_ACCOUNT_ID` | für Tunnels | Cloudflare-Account-ID; Token braucht zusätzlich Account → Cloudflare Tunnel → Read |
| `WEBHOOK_*`     | nein    | Geheimnisse für Webhooks (siehe unten)                  |
| `PORT`          | nein    | HTTP-Port, Default `8080`                               |
| `LOG_LEVEL`     | nein    | `debug`, `info`, `warn`, `error`; Default `info`        |
| `DATA_DIR`      | nein    | Verzeichnis für `app.db`, Default `/data`               |

## Entwicklung

Voraussetzungen: Go 1.27, Node 24.

```sh
# Backend (liefert ohne Frontend-Build eine Hinweisseite aus)
APP_PASSWORD=test DATA_DIR=./data go run ./cmd/server

# Frontend mit Hot Reload (Proxy auf :8080)
cd web && npm install && npm run dev
```

Ohne echtes Cloudflare-Token gegen eine nachgebaute API entwickeln
(keine echten DNS-Einträge betroffen):

```sh
go run ./cmd/cfmock -token dev -zones example.com,example.org \
  -account dev-account -tunnels home:healthy,nas:degraded      # :8787
CF_API_TOKEN=dev CF_ACCOUNT_ID=dev-account CF_API_BASE_URL=http://localhost:8787 \
  APP_PASSWORD=test DATA_DIR=./data go run ./cmd/server
curl localhost:8787/_records                              # Inhalt des Mocks
curl -X POST 'localhost:8787/_fail?status=500'             # Ausfall simulieren (status=0 beendet)
curl -X POST 'localhost:8787/_tunnel?name=home&status=down' # Tunnel-Status setzen
```

Hinweise zum Verhalten:
- Records, die bei Cloudflare fehlen, werden angelegt (Kommentar „managed by dnsdeck“).
- Bestehende Einträge werden mit ihren Cloudflare-Werten für Proxy/TTL übernommen.
- Die IP erzwingt dnsdeck; bei Proxy/TTL hat Cloudflare das letzte Wort – dnsdeck
  überträgt sie nur, wenn sie in dnsdeck geändert wurden.
- Entfernen in dnsdeck beendet nur die Verwaltung – der Eintrag bei Cloudflare bleibt.
- Im Update-Protokoll landen Anlagen, Änderungen und Fehler; gleichbleibende
  automatische Fehler nur einmal.
- Tunnels werden alle 60 s abgefragt. Gespeichert werden Zeitabschnitte gleichen
  Status (30 Tage); Zeiten ohne Abfrage bleiben in der Uptime „unbekannt“.
  `degraded` zählt als erreichbar.
- Ein dauerhafter DNS-Fehler wird einmal gemeldet, seine Behebung ebenfalls.

Lokal als Container (baut aus dem Quellcode):

```sh
cd deploy && docker compose -f docker-compose.dev.yml up -d --build
```

Checks:

```sh
go vet ./... && go test ./...
cd web && npm run lint && npm run build
```

`npm run build` schreibt nach `web/dist/`; von dort wird das Frontend per
`go:embed` ins Binary übernommen.

## Webhooks

Unter **Einstellungen → Benachrichtigungen** beliebig viele Webhooks anlegen, jeder
mit eigener Ereignisauswahl (IP-Wechsel, DNS-Update fehlgeschlagen / wieder OK,
Tunnel-Statuswechsel). Vorlagen erleichtern den Start; technisch ist jeder Webhook
gleich: Methode, URL, Header und ein Body-Template (Go `text/template`).

- Im Template: `.Type .Title .Message .Priority .Time .Data.<feld>` und die
  Funktionen `json` (für JSON-Werte, z. B. `{{json .Title}}`), `env`, `printf`,
  `upper`, `lower`, `urlquery`. Leeres Template = Standard-JSON des Ereignisses.
- **Geheimnisse** (Tokens, geheime URLs) nie direkt eintragen, sondern in
  `deploy/.env` als `WEBHOOK_…` und im Webhook als `${WEBHOOK_NAME}` (URL, Header)
  bzw. `{{env "WEBHOOK_NAME"}}` (Body). In der Datenbank steht nur der Platzhalter.
- Erlaubt sind nur Variablen mit Präfix `WEBHOOK_` – so kann über die Oberfläche
  z. B. `CF_API_TOKEN` nicht an einen fremden Server geschickt werden. Platzhalter
  werden nur im konfigurierten Text ersetzt, nie in Ereignisdaten.
- Die Vorschau im Dialog zeigt die fertige Anfrage mit Beispielereignis, ohne
  Geheimnisse. „Testen“ schickt eine echte Testnachricht.

## Releases

GitHub Actions (`.github/workflows/`):

- **CI** bei jedem Push/PR: `gofmt`, `go vet`, `go test`, Frontend-Lint und -Build.
- **Release** bei Push auf `main`: Image `ghcr.io/mrcdlm/dnsdeck:main`.
- **Release** bei Tag `vX.Y.Z`: Images `:X.Y.Z`, `:X.Y`, `:X`, `:latest` (amd64 + arm64)
  und ein GitHub-Release mit automatischen Release Notes.

```sh
git tag v1.0.0 && git push origin v1.0.0
```

Die Version erscheint unter Einstellungen → System.
