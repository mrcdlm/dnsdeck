package api

import (
	"fmt"
	"net/http"
	"time"
)

type eventSource interface {
	Subscribe() (<-chan string, func())
	Publish(topic string)
}

// SSE-Heartbeat: hält Proxys (z. B. Cloudflare Tunnel) die Verbindung offen
// und prüft dabei, ob die Session noch gültig ist.
var heartbeatInterval = 25 * time.Second

// handleEvents liefert Server-Sent Events. Jedes Ereignis nennt nur das
// geänderte Thema; der Client lädt die Daten über die API neu.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		writeError(w, http.StatusServiceUnavailable, "Live-Updates nicht verfügbar")
		return
	}
	rc := http.NewResponseController(w)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // Reverse-Proxys: nicht puffern
	w.WriteHeader(http.StatusOK)

	ch, unsubscribe := s.events.Subscribe()
	defer unsubscribe()

	// retry: Wartezeit des Browsers vor dem Neuverbinden
	fmt.Fprint(w, "retry: 5000\n: verbunden\n\n")
	if err := rc.Flush(); err != nil {
		return
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case topic, ok := <-ch:
			if !ok { // Server fährt herunter
				return
			}
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", topic)
		case <-ticker.C:
			if ok, err := s.auth.valid(r); err != nil || !ok {
				fmt.Fprint(w, "event: session\ndata: expired\n\n")
				rc.Flush()
				return
			}
			fmt.Fprint(w, ": ping\n\n")
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
