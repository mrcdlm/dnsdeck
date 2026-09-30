package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/api"
	"github.com/mrcdlm/dnsdeck/internal/config"
	"github.com/mrcdlm/dnsdeck/internal/ddns"
	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/ipdetect"
	"github.com/mrcdlm/dnsdeck/internal/providers"
	"github.com/mrcdlm/dnsdeck/internal/providers/cloudflare"
	"github.com/mrcdlm/dnsdeck/internal/scheduler"
	"github.com/mrcdlm/dnsdeck/internal/store"
	"github.com/mrcdlm/dnsdeck/internal/tunnels"
	"github.com/mrcdlm/dnsdeck/web"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("Abbruch", "err", err)
		os.Exit(1)
	}
}

// healthcheck wird vom Docker-Healthcheck aufgerufen (distroless hat kein curl).
func healthcheck() int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", config.PortFromEnv()))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "status", resp.StatusCode)
		return 1
	}
	return 0
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)
	log.Info("dnsdeck startet", "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureWritable(cfg.DataDir); err != nil {
		return err
	}
	st, err := store.Open(ctx, filepath.Join(cfg.DataDir, "app.db"))
	if err != nil {
		return fmt.Errorf("Datenbank öffnen: %w", err)
	}
	defer st.Close()

	settings, warnings := config.LoadSettings(ctx, st)
	for _, w := range warnings {
		log.Warn("Einstellung", "warning", w)
	}

	sources := ipdetect.SelectSources(ipdetect.DefaultSources(), settings.IPSources)
	broker := events.NewBroker()
	tracker := ipdetect.NewTracker(ipdetect.NewDetector(sources), st, log)
	tracker.SetPublisher(broker)
	if err := tracker.Load(ctx); err != nil {
		return fmt.Errorf("IP-Zustand laden: %w", err)
	}

	// Ohne Token läuft die App weiter; Records zeigen dann einen Fehlerstatus.
	var (
		provider providers.Provider
		zones    providers.ZoneLister
		cf       *cloudflare.Client
	)
	if cfg.CFAPIToken != "" {
		cf = cloudflare.New(cfg.CFAPIToken, cfg.CFAPIBaseURL)
		provider, zones = cf, cf
	} else {
		log.Warn("CF_API_TOKEN nicht gesetzt – DNS-Updates und Tunnel-Monitoring sind deaktiviert")
	}
	updater := ddns.NewUpdater(st, provider, log)
	updater.Pub = broker
	svc := &ddns.Service{Tracker: tracker, Updater: updater}

	var tunnelClient interface {
		ListTunnels(context.Context, string) ([]cloudflare.Tunnel, error)
	}
	if cf != nil { // kein typisiertes nil in das Interface stecken
		tunnelClient = cf
	}
	monitor := tunnels.NewMonitor(tunnelClient, cfg.CFAccountID, st, log, broker,
		func() time.Duration { return settings.TunnelInterval })
	if cf != nil && cfg.CFAccountID == "" {
		log.Warn("CF_ACCOUNT_ID nicht gesetzt – Tunnel-Monitoring ist deaktiviert")
	}

	auth, err := api.NewAuth(cfg.AppPassword, st)
	if err != nil {
		return err
	}
	cfg.AppPassword = "" // wird ab hier nicht mehr benötigt

	sched := scheduler.New(log)
	sched.Add(scheduler.Job{
		Name:     "ddns",
		Interval: func() time.Duration { return settings.IPCheckInterval },
		Run: func(ctx context.Context) error {
			_, err := svc.RunCycle(ctx, store.TriggerScheduled)
			return err
		},
	})
	if monitor.Configured() {
		sched.Add(scheduler.Job{
			Name:     "tunnels",
			Interval: func() time.Duration { return settings.TunnelInterval },
			Run:      monitor.Poll,
		})
	}
	sched.Add(scheduler.Job{
		Name:     "session-cleanup",
		Interval: func() time.Duration { return time.Hour },
		Run: func(ctx context.Context) error {
			_, err := st.PurgeExpiredSessions(ctx, time.Now())
			return err
		},
	})

	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port),
		Handler: api.NewServer(api.Deps{
			Store: st, Tracker: tracker, DDNS: svc, Zones: zones,
			Tunnels: monitor, Events: broker,
			Auth: auth, Log: log, Static: web.Dist(),
		}).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("HTTP-Server lauscht", "addr", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	sched.Start(ctx)

	select {
	case <-ctx.Done():
		log.Info("Beende …")
	case err := <-errCh:
		stop()
		sched.Wait()
		return err
	}

	// SSE-Verbindungen zuerst beenden – Shutdown wartet sonst auf sie.
	broker.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Shutdown", "err", err)
	}
	sched.Wait()
	return nil
}

// ensureWritable liefert eine verständliche Meldung, wenn das Datenverzeichnis
// fehlt oder (typisch bei Bind-Mounts) dem Container-User nicht gehört.
func ensureWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("Datenverzeichnis %s nicht anlegbar: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return fmt.Errorf("Datenverzeichnis %s nicht beschreibbar (UID %d) – "+
			"auf dem Host z. B. 'sudo chown %d:%d <pfad>/data' ausführen: %w",
			dir, os.Getuid(), os.Getuid(), os.Getgid(), err)
	}
	f.Close()
	return os.Remove(f.Name())
}
