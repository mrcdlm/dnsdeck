package api

import (
	"context"
	"net/http"

	"github.com/mrcdlm/dnsdeck/internal/tunnels"
)

type tunnelService interface {
	Configured() bool
	Poll(ctx context.Context) error
	Overview(ctx context.Context) (tunnels.Overview, error)
}

func (s *Server) handleTunnels(w http.ResponseWriter, r *http.Request) {
	if s.tunnels == nil {
		writeJSON(w, http.StatusOK, tunnels.Overview{Tunnels: []tunnels.TunnelView{}})
		return
	}
	o, err := s.tunnels.Overview(r.Context())
	if err != nil {
		s.internalError(w, "Tunnels lesen", err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// handleTunnelsRefresh fragt sofort ab. Ein Cloudflare-Fehler ist kein
// HTTP-Fehler: er steht in Overview.Error, der letzte Stand bleibt sichtbar.
func (s *Server) handleTunnelsRefresh(w http.ResponseWriter, r *http.Request) {
	if s.tunnels == nil || !s.tunnels.Configured() {
		writeError(w, http.StatusServiceUnavailable, "Tunnel-Monitoring nicht konfiguriert (CF_API_TOKEN/CF_ACCOUNT_ID)")
		return
	}
	ctx, cancel := detached(r)
	defer cancel()
	if err := s.tunnels.Poll(ctx); err != nil {
		s.log.Warn("Tunnel-Abfrage fehlgeschlagen", "err", err)
	}
	s.handleTunnels(w, r)
}
