// Command xiherald serves the FFXI Herald: a read-only statistics site over a
// LandSandBoat game database.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JasonPulse/xiherald/internal/config"
	"github.com/JasonPulse/xiherald/internal/web"
	"github.com/JasonPulse/xiherald/internal/xidb"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.Load()

	db, err := xidb.Open(cfg.DSN(), cfg.CacheTTL)
	if err != nil {
		return err
	}
	defer db.Close()

	// The database is allowed to be down at start: the Herald comes up, /healthz
	// reports unready, and pages recover on their own once it is back. That
	// keeps a Herald restart from depending on game server start order.
	startupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.Ping(startupCtx); err != nil {
		log.Warn("database not reachable at startup, continuing",
			"host", cfg.DBHost, "db", cfg.DBName, "err", err)
	} else {
		log.Info("database connected", "host", cfg.DBHost, "db", cfg.DBName)
	}

	portraits, err := db.OpenPortraits(startupCtx, cfg.PortraitSchema)
	if err != nil {
		// Portraits are a decoration. The Herald serves every page without
		// them, so this is a warning rather than a failed start.
		log.Warn("portraits unavailable", "schema", cfg.PortraitSchema, "err", err)
	} else if portraits.Enabled() {
		log.Info("portraits enabled", "schema", cfg.PortraitSchema)
	} else {
		log.Info("portraits disabled by configuration")
	}

	srv, err := web.New(db, cfg.ServerName, portraits, log)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           requestLog(log, srv),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("herald listening", "addr", cfg.Addr, "server", cfg.ServerName)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errs:
		return err
	case sig := <-stop:
		log.Info("shutting down", "signal", sig.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	return httpServer.Shutdown(shutdownCtx)
}

// requestLog records completed requests. Probe endpoints are skipped so
// Kubernetes does not fill the log.
func requestLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/livez" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path,
			"took", time.Since(start).Round(time.Millisecond).String())
	})
}
