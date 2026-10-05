// Package api stellt die HTTP-Schnittstelle und das eingebettete Frontend bereit.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/ipdetect"
	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/providers"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

type dataStore interface {
	sessionStore
	Ping(ctx context.Context) error
	ListIPChanges(ctx context.Context, f store.IPChangeFilter, limit int) ([]store.IPChange, error)
	TunnelStatusChanges(ctx context.Context, f store.TunnelChangeFilter, limit int) ([]store.TunnelChange, error)
	recordStore
	webhookStore
	probeStore
	ListSpeedtests(ctx context.Context, beforeID int64, limit int) ([]store.Speedtest, error)
}

type ipTracker interface {
	State() ipdetect.State
}

// ddnsService führt IP-Prüfung und DNS-Abgleich aus.
type ddnsService interface {
	RunCycle(ctx context.Context, trigger string) (ipdetect.State, error)
	SyncRecord(ctx context.Context, id int64, trigger string) (store.Record, error)
}

// Deps sind die Abhängigkeiten des HTTP-Servers.
type Deps struct {
	Store   dataStore
	Tracker ipTracker
	DDNS    ddnsService
	Zones   providers.ZoneLister // nil = Cloudflare nicht konfiguriert
	Tunnels tunnelService        // nil = kein Tunnel-Monitoring
	Events  eventSource          // nil = keine Live-Updates
	// Settings: Laufzeit-Einstellungen (nil = nicht änderbar)
	Settings settingsService
	// Webhooks: Zustellung von Testnachrichten (nil = keine Webhooks)
	Webhooks webhookTester
	// WebhookEnv: Zugriff auf Env-Variablen, um fehlende WEBHOOK_* anzuzeigen
	WebhookEnv notify.Env
	// Propagation: DNS-Verbreitungsprüfung (nil = abgeschaltet)
	Propagation PropagationChecker
	// Probes: Erreichbarkeitsprüfungen sofort ausführen (nil = nur speichern)
	Probes ProbeRunner
	// Speedtest: Geschwindigkeitsmessung (nil = nicht verfügbar)
	Speedtest SpeedtestRunner
	// TrustedProxies: Proxys, deren Client-IP-Header gelten (leer = keine)
	TrustedProxies []netip.Prefix
	Info           Info
	Auth           *Auth
	Log            *slog.Logger
	Static         fs.FS
}

type Server struct {
	store          dataStore
	tracker        ipTracker
	ddns           ddnsService
	zones          providers.ZoneLister
	tunnels        tunnelService
	events         eventSource
	settings       settingsService
	webhooks       webhookTester
	webhookEnv     notify.Env
	propagation    PropagationChecker
	probes         ProbeRunner
	speedtest      SpeedtestRunner
	trustedProxies []netip.Prefix
	info           Info
	auth           *Auth
	log            *slog.Logger
	static         fs.FS
}

func NewServer(d Deps) *Server {
	return &Server{store: d.Store, tracker: d.Tracker, ddns: d.DDNS, zones: d.Zones,
		tunnels: d.Tunnels, events: d.Events, settings: d.Settings, webhooks: d.Webhooks,
		webhookEnv: d.WebhookEnv, propagation: d.Propagation, probes: d.Probes, speedtest: d.Speedtest, trustedProxies: d.TrustedProxies, info: d.Info, auth: d.Auth, log: d.Log, static: d.Static}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Get("/healthz", s.handleHealth)

	r.Route("/api", func(r chi.Router) {
		r.Use(noStore)
		// CSRF: schreibende Anfragen nur von derselben Origin. SameSite=Strict
		// allein schützt nicht vor Seiten auf Nachbar-Subdomains (gleiche Site).
		r.Use(http.NewCrossOriginProtection().Handler)
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
		r.Get("/session", s.handleSession)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/ip", s.handleIP)
			r.Get("/ip/history", s.handleIPHistory)
			r.Post("/ip/refresh", s.handleIPRefresh)

			r.Get("/zones", s.handleZones)
			r.Get("/records", s.handleListRecords)
			r.Post("/records", s.handleCreateRecord)
			r.Post("/records/sync", s.handleSyncAll)
			r.Put("/records/{id}", s.handleUpdateRecord)
			r.Delete("/records/{id}", s.handleDeleteRecord)
			r.Post("/records/{id}/sync", s.handleSyncRecord)
			r.Post("/records/{id}/propagation", s.handleCheckPropagation)
			r.Get("/updates", s.handleUpdateLog)

			r.Get("/tunnels", s.handleTunnels)
			r.Post("/tunnels/refresh", s.handleTunnelsRefresh)
			r.Get("/tunnels/history", s.handleTunnelHistory)
			r.Get("/events", s.handleEvents)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Get("/webhooks", s.handleListWebhooks)
			r.Post("/webhooks", s.handleCreateWebhook)
			r.Post("/webhooks/preview", s.handlePreviewWebhook)
			r.Put("/webhooks/{id}", s.handleUpdateWebhook)
			r.Delete("/webhooks/{id}", s.handleDeleteWebhook)
			r.Post("/webhooks/{id}/test", s.handleTestWebhook)
			r.Get("/probes", s.handleListProbes)
			r.Post("/probes", s.handleCreateProbe)
			r.Put("/probes/{id}", s.handleUpdateProbe)
			r.Delete("/probes/{id}", s.handleDeleteProbe)
			r.Post("/probes/{id}/run", s.handleRunProbe)
			r.Get("/speedtest", s.handleSpeedtest)
			r.Post("/speedtest/run", s.handleRunSpeedtest)
			r.Get("/info", s.handleInfo)
		})

		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			writeMsg(w, req, http.StatusNotFound, i18n.M("api.not_found"))
		})
	})

	r.NotFound(spaHandler(s.static).ServeHTTP)
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		s.log.Error("healthz: DB nicht erreichbar", "err", err)
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("ok"))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	client := s.clientIP(r)
	if !s.auth.beginAttempt(client) {
		writeMsg(w, r, http.StatusTooManyRequests, i18n.M("auth.too_many_attempts"))
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.bad_request"))
		return
	}

	if !s.auth.checkPassword(body.Password) {
		s.log.Warn("Login fehlgeschlagen", "client", client)
		select {
		case <-time.After(s.auth.delay):
		case <-r.Context().Done():
		}
		writeMsg(w, r, http.StatusUnauthorized, i18n.M("auth.wrong_password"))
		return
	}

	s.auth.loginSucceeded(client)
	token, exp, err := s.auth.newSession(r.Context())
	if err != nil {
		s.log.Error("Session anlegen fehlgeschlagen", "err", err)
		writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.internal"))
		return
	}
	http.SetCookie(w, sessionCookieFor(r, token, exp))
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := s.store.DeleteSession(r.Context(), s.auth.hashToken(c.Value)); err != nil {
			s.log.Error("Session löschen fehlgeschlagen", "err", err)
		}
	}
	http.SetCookie(w, sessionCookieFor(r, "", time.Time{}))
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	ok, err := s.auth.valid(r)
	if errors.Is(err, context.Canceled) {
		return // Browser hat die Anfrage abgebrochen
	}
	if err != nil {
		s.log.Error("Session prüfen fehlgeschlagen", "err", err)
		writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.internal"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": ok})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, err := s.auth.valid(r)
		if errors.Is(err, context.Canceled) {
			return // Browser hat die Anfrage abgebrochen (z. B. Seitenwechsel)
		}
		if err != nil {
			s.log.Error("Session prüfen fehlgeschlagen", "err", err)
			writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.internal"))
			return
		}
		if !ok {
			writeMsg(w, r, http.StatusUnauthorized, i18n.M("api.unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.tracker.State().Localize(i18n.FromRequest(r)))
}

func (s *Server) handleIPRefresh(w http.ResponseWriter, r *http.Request) {
	// IP prüfen und alle Einträge abgleichen; auch dann zu Ende führen, wenn der Browser die Verbindung schließt.
	ctx, cancel := detached(r)
	defer cancel()
	state, err := s.ddns.RunCycle(ctx, store.TriggerManual)
	if err != nil {
		s.log.Error("manueller Durchlauf fehlgeschlagen", "err", err)
		writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.refresh_failed"))
		return
	}
	writeJSON(w, http.StatusOK, state.Localize(i18n.FromRequest(r)))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeMsg schreibt eine Fehlerantwort in der Sprache der Anfrage. Neben dem
// übersetzten Text stehen Code und Parameter (für Clients und Tests).
func writeMsg(w http.ResponseWriter, r *http.Request, status int, m i18n.Msg) {
	writeJSON(w, status, map[string]any{"error": i18n.T(i18n.FromRequest(r), m), "code": m.Code, "params": m.Params})
}

// writeMsgs schreibt mehrere Meldungen (z. B. Validierungsfehler) als eine Antwort.
func writeMsgs(w http.ResponseWriter, r *http.Request, status int, msgs []i18n.Msg) {
	writeMsg(w, r, status, i18n.JoinMsgs(msgs...))
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "+
				"connect-src 'self'; manifest-src 'self'; worker-src 'self'; "+
				"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
