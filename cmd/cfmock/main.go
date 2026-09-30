// cfmock startet eine nachgebaute Cloudflare-API für die lokale Entwicklung,
// damit dnsdeck ohne echtes Token und ohne echte DNS-Einträge getestet werden
// kann. Nicht Teil des Docker-Images.
//
//	go run ./cmd/cfmock -addr :8787 -token dev -zones example.com,example.org \
//	  -account dev-account -tunnels home:healthy,nas:degraded
//	CF_API_TOKEN=dev CF_ACCOUNT_ID=dev-account CF_API_BASE_URL=http://localhost:8787 \
//	  APP_PASSWORD=test go run ./cmd/server
//
// Steuerung (ohne Token):
//
//	GET  /_records                          aktueller Inhalt
//	POST /_fail?status=500                  Ausfälle simulieren (status=0 beendet)
//	POST /_tunnel?name=home&status=down     Tunnel-Status setzen (healthy|degraded|down|inactive)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare/cftest"
)

func main() {
	addr := flag.String("addr", ":8787", "Listen-Adresse")
	token := flag.String("token", "dev", "erwartetes API-Token")
	zoneList := flag.String("zones", "example.com", "kommagetrennte Zonen")
	account := flag.String("account", "dev-account", "Account-ID für Tunnels")
	tunnelList := flag.String("tunnels", "home:healthy", "kommagetrennte Tunnels name:status")
	flag.Parse()

	var zones []cftest.Zone
	for i, name := range strings.Split(*zoneList, ",") {
		if name = strings.TrimSpace(name); name != "" {
			zones = append(zones, cftest.Zone{ID: fmt.Sprintf("zone%d", i+1), Name: name})
		}
	}
	fake := cftest.New(*token, zones...)
	fake.SetAccount(*account)
	for i, spec := range strings.Split(*tunnelList, ",") {
		name, status, _ := strings.Cut(strings.TrimSpace(spec), ":")
		if name == "" {
			continue
		}
		if status == "" {
			status = "healthy"
		}
		fake.AddTunnel(fmt.Sprintf("tunnel-%d", i+1), name, status)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_records", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(fake.Records())
	})
	// POST /_fail?status=500 simuliert Ausfälle, status=0 beendet sie.
	mux.HandleFunc("POST /_fail", func(w http.ResponseWriter, r *http.Request) {
		var status int
		fmt.Sscan(r.URL.Query().Get("status"), &status)
		fake.Fail(status)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_tunnel", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("status") {
		case "healthy", "degraded", "down", "inactive":
		default:
			http.Error(w, "status muss healthy|degraded|down|inactive sein", http.StatusBadRequest)
			return
		}
		if !fake.SetTunnelStatus(q.Get("name"), q.Get("status")) {
			http.Error(w, "Tunnel unbekannt", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", fake)

	log.Printf("Cloudflare-Mock auf %s, Zonen: %s, Account: %s, Tunnels: %s", *addr, *zoneList, *account, *tunnelList)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
