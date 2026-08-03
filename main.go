// Command drop ist ein ephemeres Transfer-Tool: Räume leben nur im RAM,
// Text und Dateien werden über WebSockets zwischen den Mitgliedern relayt.
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

	"github.com/doenke/drop/internal/config"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("Start fehlgeschlagen", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.SessionKeyEphemeral {
		log.Warn("DROP_SESSION_KEY nicht gesetzt — Schlüssel zur Laufzeit erzeugt, Sessions überleben keinen Neustart")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mountStatic(mux)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           withRecover(log, mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("drop läuft", "addr", cfg.Addr, "public_url", cfg.PublicURL.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("Herunterfahren")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// withRecover verhindert, dass ein Panic in einem Handler den ganzen Server
// mitnimmt — bei einem Tool, das dauerhaft WebSockets hält, ist das wichtig.
func withRecover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("Panic im Handler", "path", r.URL.Path, "panic", v)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
