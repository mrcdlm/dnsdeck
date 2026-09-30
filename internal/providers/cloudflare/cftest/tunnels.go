package cftest

import (
	"net/http"
	"strconv"
	"time"
)

type Connection struct {
	ID                 string    `json:"id"`
	ColoName           string    `json:"colo_name"`
	ClientID           string    `json:"client_id"`
	ClientVersion      string    `json:"client_version"`
	OpenedAt           time.Time `json:"opened_at"`
	OriginIP           string    `json:"origin_ip"`
	IsPendingReconnect bool      `json:"is_pending_reconnect"`
}

type Tunnel struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Status          string       `json:"status"` // inactive | degraded | healthy | down
	CreatedAt       time.Time    `json:"created_at"`
	DeletedAt       *time.Time   `json:"deleted_at"`
	ConnsActiveAt   *time.Time   `json:"conns_active_at"`
	ConnsInactiveAt *time.Time   `json:"conns_inactive_at"`
	TunType         string       `json:"tun_type"`
	RemoteConfig    bool         `json:"remote_config"`
	Connections     []Connection `json:"connections"`
}

// SetAccount legt die Account-ID fest, unter der Tunnels abrufbar sind.
func (f *Fake) SetAccount(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accountID = id
}

// AddTunnel fügt einen Tunnel hinzu; Status und Verbindungen setzt SetTunnelStatus.
func (f *Fake) AddTunnel(id, name, status string) {
	f.mu.Lock()
	t := &Tunnel{ID: id, Name: name, TunType: "cfd_tunnel", RemoteConfig: true,
		CreatedAt: time.Now().Add(-30 * 24 * time.Hour), Status: "inactive", Connections: []Connection{}}
	f.tunnels = append(f.tunnels, t)
	f.mu.Unlock()
	f.SetTunnelStatus(id, status)
}

// SetTunnelStatus setzt den Status wie cloudflared ihn erzeugen würde:
// healthy = 4 Verbindungen, degraded = 1, down/inactive = keine.
func (f *Fake) SetTunnelStatus(id, status string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tunnels {
		if t.ID != id && t.Name != id {
			continue
		}
		now := time.Now().UTC()
		wasUp := len(t.Connections) > 0
		t.Status = status
		n := map[string]int{"healthy": 4, "degraded": 1}[status]
		colos := []string{"fra06", "ams01", "fra10", "dus01"}
		t.Connections = []Connection{}
		for i := range n {
			t.Connections = append(t.Connections, Connection{
				ID: t.ID + "-c" + strconv.Itoa(i), ColoName: colos[i], ClientID: t.ID + "-client",
				ClientVersion: "2026.9.0", OpenedAt: now, OriginIP: "203.0.113.10",
			})
		}
		switch {
		case n > 0 && !wasUp:
			t.ConnsActiveAt, t.ConnsInactiveAt = &now, nil
		case n == 0 && wasUp:
			t.ConnsInactiveAt = &now
		}
		return true
	}
	return false
}

// RemoveTunnel entfernt einen Tunnel (wie is_deleted=true).
func (f *Fake) RemoveTunnel(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, t := range f.tunnels {
		if t.ID == id {
			f.tunnels = append(f.tunnels[:i], f.tunnels[i+1:]...)
			return
		}
	}
}

func (f *Fake) listTunnels(w http.ResponseWriter, r *http.Request, account string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.accountID == "" || account != f.accountID {
		writeErr(w, http.StatusForbidden, 10000, "Authentication error")
		return
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 {
		perPage = 20
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	all := make([]Tunnel, 0, len(f.tunnels))
	for _, t := range f.tunnels {
		all = append(all, *t)
	}
	start := min((page-1)*perPage, len(all))
	end := min(start+perPage, len(all))
	total := max(1, (len(all)+perPage-1)/perPage)
	writeOK(w, all[start:end], &resultInfo{Page: page, TotalPages: total})
}
