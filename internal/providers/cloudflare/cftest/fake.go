// Package cftest stellt eine nachgebaute Cloudflare-API (Teilmenge) für Tests
// und lokale Entwicklung bereit. Es werden nur die von dnsdeck genutzten
// Endpunkte unterstützt.
package cftest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Record struct {
	ID      string `json:"id"`
	ZoneID  string `json:"zone_id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment,omitempty"`
}

// Fake ist ein http.Handler, der die Cloudflare-API nachbildet.
type Fake struct {
	Token string

	mu      sync.Mutex
	zones   []Zone
	records map[string]*Record
	nextID  atomic.Int64
	// FailWith: wenn gesetzt, beantwortet der Fake jede Anfrage mit diesem Status.
	failWith int
	// Zähler für Tests
	Gets, Creates, Patches atomic.Int64
}

func New(token string, zones ...Zone) *Fake {
	return &Fake{Token: token, zones: zones, records: map[string]*Record{}}
}

// AddRecord legt einen Eintrag direkt an (Testvorbereitung) und liefert die ID.
func (f *Fake) AddRecord(r Record) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.ID = fmt.Sprintf("rec%d", f.nextID.Add(1))
	f.records[r.ID] = &r
	return r.ID
}

// Patch ändert einen Eintrag direkt (simuliert Änderungen im Cloudflare-Dashboard).
func (f *Fake) Patch(id string, fn func(*Record)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.records[id]; ok {
		fn(r)
	}
}

// Records liefert eine Kopie aller Einträge, sortiert nach Name/Typ.
func (f *Fake) Records() []Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Record, 0, len(f.records))
	for _, r := range f.records {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// Fail lässt alle folgenden Anfragen mit status scheitern (0 = normal).
func (f *Fake) Fail(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failWith = status
}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+f.Token {
		writeErr(w, http.StatusForbidden, 10000, "Authentication error")
		return
	}
	f.mu.Lock()
	fail := f.failWith
	f.mu.Unlock()
	if fail != 0 {
		writeErr(w, fail, 10001, "simulierter Fehler")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// Optionales Präfix "client/v4" wie bei der echten API
	if len(parts) >= 2 && parts[0] == "client" && parts[1] == "v4" {
		parts = parts[2:]
	}
	switch {
	case len(parts) == 1 && parts[0] == "zones" && r.Method == http.MethodGet:
		f.listZones(w)
	case len(parts) == 3 && parts[0] == "zones" && parts[2] == "dns_records":
		switch r.Method {
		case http.MethodGet:
			f.listRecords(w, r, parts[1])
		case http.MethodPost:
			f.createRecord(w, r, parts[1])
		default:
			writeErr(w, http.StatusMethodNotAllowed, 10000, "method not allowed")
		}
	case len(parts) == 4 && parts[0] == "zones" && parts[2] == "dns_records" && r.Method == http.MethodPatch:
		f.patchRecord(w, r, parts[1], parts[3])
	default:
		writeErr(w, http.StatusNotFound, 7003, "No route for that URI")
	}
}

func (f *Fake) hasZone(id string) bool {
	for _, z := range f.zones {
		if z.ID == id {
			return true
		}
	}
	return false
}

func (f *Fake) listZones(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	writeOK(w, f.zones, &resultInfo{Page: 1, TotalPages: 1})
}

func (f *Fake) listRecords(w http.ResponseWriter, r *http.Request, zone string) {
	f.Gets.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasZone(zone) {
		writeErr(w, http.StatusNotFound, 7003, "Could not route to zone")
		return
	}
	q := r.URL.Query()
	out := []Record{}
	for _, rec := range f.records {
		if rec.ZoneID != zone {
			continue
		}
		if t := q.Get("type"); t != "" && rec.Type != t {
			continue
		}
		if n := q.Get("name.exact"); n != "" && rec.Name != n {
			continue
		}
		out = append(out, *rec)
	}
	writeOK(w, out, &resultInfo{Page: 1, TotalPages: 1})
}

func (f *Fake) createRecord(w http.ResponseWriter, r *http.Request, zone string) {
	f.Creates.Add(1)
	var in Record
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, 9207, "Request body is invalid")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasZone(zone) {
		writeErr(w, http.StatusNotFound, 7003, "Could not route to zone")
		return
	}
	if msg := validate(in); msg != "" {
		writeErr(w, http.StatusBadRequest, 9005, msg)
		return
	}
	in.ID = fmt.Sprintf("rec%d", f.nextID.Add(1))
	in.ZoneID = zone
	in.TTL = normalizeTTL(in)
	f.records[in.ID] = &in
	writeOK(w, in, nil)
}

func (f *Fake) patchRecord(w http.ResponseWriter, r *http.Request, zone, id string) {
	f.Patches.Add(1)
	var in map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, 9207, "Request body is invalid")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[id]
	if !ok || rec.ZoneID != zone {
		writeErr(w, http.StatusNotFound, 81044, "Record does not exist.")
		return
	}
	upd := *rec
	for k, v := range in {
		switch k {
		case "content":
			json.Unmarshal(v, &upd.Content)
		case "ttl":
			json.Unmarshal(v, &upd.TTL)
		case "proxied":
			json.Unmarshal(v, &upd.Proxied)
		case "name":
			json.Unmarshal(v, &upd.Name)
		case "type":
			json.Unmarshal(v, &upd.Type)
		}
	}
	if msg := validate(upd); msg != "" {
		writeErr(w, http.StatusBadRequest, 9005, msg)
		return
	}
	upd.TTL = normalizeTTL(upd)
	*rec = upd
	writeOK(w, upd, nil)
}

func validate(r Record) string {
	a, err := netip.ParseAddr(r.Content)
	switch {
	case r.Type != "A" && r.Type != "AAAA":
		return "unsupported record type"
	case err != nil, r.Type == "A" && !a.Is4(), r.Type == "AAAA" && (!a.Is6() || a.Is4In6()):
		return "Content for " + r.Type + " record is invalid."
	case r.TTL != 1 && (r.TTL < 60 || r.TTL > 86400):
		return "TTL must be between 60 and 86400 seconds, or 1 for Automatic."
	}
	return ""
}

// Wie bei Cloudflare: proxied Einträge haben immer TTL "automatisch".
func normalizeTTL(r Record) int {
	if r.Proxied {
		return 1
	}
	return r.TTL
}

type resultInfo struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

func writeOK(w http.ResponseWriter, result any, info *resultInfo) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true, "errors": []any{}, "messages": []any{},
		"result": result, "result_info": info,
	})
}

func writeErr(w http.ResponseWriter, status, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"success": false, "result": nil, "messages": []any{},
		"errors": []map[string]any{{"code": code, "message": msg}},
	})
}
