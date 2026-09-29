// cfmock startet eine nachgebaute Cloudflare-API für die lokale Entwicklung,
// damit dnsdeck ohne echtes Token und ohne echte DNS-Einträge getestet werden
// kann. Nicht Teil des Docker-Images.
//
//	go run ./cmd/cfmock -addr :8787 -token dev -zones example.com,example.org
//	CF_API_TOKEN=dev CF_API_BASE_URL=http://localhost:8787 APP_PASSWORD=test go run ./cmd/server
//
// GET /_records zeigt den aktuellen Inhalt (ohne Token).
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
	flag.Parse()

	var zones []cftest.Zone
	for i, name := range strings.Split(*zoneList, ",") {
		if name = strings.TrimSpace(name); name != "" {
			zones = append(zones, cftest.Zone{ID: fmt.Sprintf("zone%d", i+1), Name: name})
		}
	}
	fake := cftest.New(*token, zones...)

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
	mux.Handle("/", fake)

	log.Printf("Cloudflare-Mock auf %s, Zonen: %s", *addr, *zoneList)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
