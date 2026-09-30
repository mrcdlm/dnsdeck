#!/bin/sh
# Baut Frontend und Server und startet dnsdeck für die Browser-Tests
# (frische Datenbank in einem temporären Verzeichnis).
set -eu
cd "$(dirname "$0")"

[ -d ../web/node_modules ] || npm --prefix ../web ci
npm --prefix ../web run build >/dev/null
mkdir -p .tmp
go build -o .tmp/server ../cmd/server

DATA_DIR="$(mktemp -d)"
export DATA_DIR
export APP_PASSWORD=test PORT=18080 LOG_LEVEL=info
export CF_API_TOKEN=dev CF_ACCOUNT_ID=dev-account CF_API_BASE_URL=http://localhost:8787
# Verbreitungsprüfung gegen die DNS-Server des Mocks (aktuell / 15 s verzögert)
export DNSCHECK_RESOLVERS='Aktuell=127.0.0.1:8553,Verzögert=127.0.0.1:8554' DNSCHECK_AUTHORITATIVE=off
# Werte für Webhook-Platzhalter in den Tests
export WEBHOOK_TEST_PATH=geheimer-pfad WEBHOOK_CHAT=42
exec .tmp/server
