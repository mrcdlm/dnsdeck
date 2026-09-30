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
//
// DNS (UDP) für die Verbreitungsprüfung, z. B. DNSCHECK_RESOLVERS=Aktuell=127.0.0.1:8553,Verzögert=127.0.0.1:8554
// DNSCHECK_AUTHORITATIVE=off:
//
//	-dns :8553          liefert den aktuellen Stand der Mock-Records
//	-dns-lagged :8554   liefert den Stand von vor -dns-lag (wie ein Resolver-Cache)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare/cftest"
)

func main() {
	addr := flag.String("addr", ":8787", "Listen-Adresse")
	token := flag.String("token", "dev", "erwartetes API-Token")
	zoneList := flag.String("zones", "example.com", "kommagetrennte Zonen")
	account := flag.String("account", "dev-account", "Account-ID für Tunnels")
	tunnelList := flag.String("tunnels", "home:healthy", "kommagetrennte Tunnels name:status")
	dnsAddr := flag.String("dns", "", "DNS-Server mit aktuellem Stand (leer = aus), z. B. :8553")
	dnsLaggedAddr := flag.String("dns-lagged", "", "DNS-Server mit verzögertem Stand (leer = aus), z. B. :8554")
	dnsLag := flag.Duration("dns-lag", 20*time.Second, "Verzögerung für -dns-lagged")
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

	if *dnsAddr != "" {
		serveDNS(*dnsAddr, &dnsView{fake: fake})
		log.Printf("DNS (aktuell) auf %s/udp", *dnsAddr)
	}
	if *dnsLaggedAddr != "" {
		serveDNS(*dnsLaggedAddr, &dnsView{fake: fake, lag: *dnsLag})
		log.Printf("DNS (%s verzögert) auf %s/udp", *dnsLag, *dnsLaggedAddr)
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
