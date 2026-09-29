# dnsdeck

Selbst gehosteter DDNS-Updater mit Web-Dashboard und Cloudflare-Tunnel-Monitoring –
ein Go-Binary mit eingebettetem React-Frontend, ausgeliefert als ein Container.

**Stand:** Meilenstein 1 (Grundgerüst): Erkennung der öffentlichen IPv4/IPv6 per
Mehrheitsentscheid mehrerer Quellen, IP-Verlauf in SQLite, Dashboard mit Login.

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
| `CF_API_TOKEN`  | ab M2   | Cloudflare-API-Token                                    |
| `CF_ACCOUNT_ID` | ab M3   | Cloudflare-Account-ID                                   |
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

Checks:

```sh
go vet ./... && go test ./...
cd web && npm run lint && npm run build
```

`npm run build` schreibt nach `web/dist/`; von dort wird das Frontend per
`go:embed` ins Binary übernommen.
