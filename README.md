# dnsdeck

Selbst gehosteter DDNS-Updater mit Web-Dashboard und Cloudflare-Tunnel-Monitoring –
ein Go-Binary mit eingebettetem React-Frontend, ausgeliefert als ein Container.

**Stand:** Meilenstein 3
- Erkennung der öffentlichen IPv4/IPv6 per Mehrheitsentscheid mehrerer Quellen, IP-Verlauf
- DDNS mit Cloudflare: A-/AAAA-Records verwalten (Proxy, TTL), automatischer Abgleich
  (nur bei Abweichung vom Ist-Zustand), Update-Protokoll
- Tunnel-Monitoring: Status, Verbindungen, Colos, Uptime-Balken 24 h / 7 Tage
- Live-Updates per Server-Sent Events (`/api/events`)
- Dashboard, Records- und Tunnels-Seite, Login

## Betrieb mit Docker Compose

```sh
cd deploy
cp .env.example .env              # Werte eintragen
mkdir -p data && sudo chown 65532:65532 data   # Container läuft als UID 65532
docker compose up -d --build
```

Danach ist das Dashboard unter <http://localhost:8080> erreichbar
(Healthcheck: `/healthz`).

| Variable        | Pflicht | Beschreibung                                            |
|-----------------|---------|---------------------------------------------------------|
| `APP_PASSWORD`  | ja      | Dashboard-Passwort (Klartext oder bcrypt-Hash)          |
| `CF_API_TOKEN`  | für DDNS | Cloudflare-API-Token (Zone → DNS → Edit, Zone → Zone → Read) |
| `CF_ACCOUNT_ID` | für Tunnels | Cloudflare-Account-ID; Token braucht zusätzlich Account → Cloudflare Tunnel → Read |
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

Checks:

```sh
go vet ./... && go test ./...
cd web && npm run lint && npm run build
```

`npm run build` schreibt nach `web/dist/`; von dort wird das Frontend per
`go:embed` ins Binary übernommen.
