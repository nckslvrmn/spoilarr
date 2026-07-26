// Command spoilarr is a self-hosted service that hides Plex spoilers: it blurs
// or replaces episode/season artwork, clears or rewrites descriptions, and
// masks episode titles for shows you opt in — automatically as new items are
// added, and reversibly per a configurable reveal policy.
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

	"github.com/nckslvrmn/spoilarr/internal/config"
	"github.com/nckslvrmn/spoilarr/internal/poller"
	"github.com/nckslvrmn/spoilarr/internal/store"
	"github.com/nckslvrmn/spoilarr/internal/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel()}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.Load()
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}
	log.Info("starting spoilarr", "addr", cfg.Addr, "data_dir", cfg.DataDir)

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()

	srv, err := web.New(cfg, st, log)
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run the HTTP server until a shutdown signal arrives, then drain.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Background poller: the webhook backstop that protects new episodes and
	// applies reveal policies.
	pol := poller.New(st, cfg, log, cfg.PollInterval)
	go pol.Run(ctx)

	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutdown signal received, draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
}

func logLevel() slog.Level {
	switch os.Getenv("SPOILARR_LOG_LEVEL") {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
