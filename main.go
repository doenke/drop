// Command drop ist ein ephemeres Transfer-Tool: Räume leben nur im RAM,
// Text und Dateien werden über WebSockets zwischen den Mitgliedern relayt.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/doenke/drop/internal/auth"
	"github.com/doenke/drop/internal/avatar"
	"github.com/doenke/drop/internal/config"
	"github.com/doenke/drop/internal/httpx"
	"github.com/doenke/drop/internal/qr"
	"github.com/doenke/drop/internal/room"
	"github.com/doenke/drop/internal/ws"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false,
		"fragt /healthz auf der eigenen Adresse ab und beendet sich mit 0 oder 1")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	// Das Image enthält weder Shell noch curl, deshalb prüft das Binary sich
	// selbst — so kommt der Container-Healthcheck ohne zusätzliche Werkzeuge
	// aus.
	if *healthcheck {
		if err := probe(); err != nil {
			log.Error("Healthcheck fehlgeschlagen", "err", err)
			os.Exit(1)
		}
		return
	}

	if err := run(log); err != nil {
		log.Error("Start fehlgeschlagen", "err", err)
		os.Exit(1)
	}
}

// probe fragt /healthz auf der Adresse ab, auf der der Server im selben
// Container lauscht.
func probe() error {
	addr := os.Getenv("DROP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("DROP_ADDR ist keine gültige Adresse: %q", addr)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return err
	}
	// Bewusst ohne Proxy aus der Umgebung: die Anfrage geht an den eigenen
	// Container, ein gesetztes HTTP_PROXY würde sie nur ins Leere schicken.
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("/healthz antwortet mit %s", resp.Status)
	}
	return nil
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.SessionKeyEphemeral {
		log.Warn("DROP_SESSION_KEY nicht gesetzt — Schlüssel zur Laufzeit erzeugt, Sessions überleben keinen Neustart")
	}

	signer := auth.NewSigner(cfg.SessionKey, cfg.SessionTTL, cfg.CookieSecure())

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStartup()
	oidcAuth, err := auth.NewOIDC(startupCtx, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret,
		cfg.RedirectURL(), cfg.OIDCScopes, signer, log)
	if err != nil {
		return err
	}

	hub := room.NewHub(room.Options{
		EmptyGrace: cfg.RoomEmptyGrace,
		MaxAge:     cfg.RoomMaxAge,
		MaxMembers: cfg.RoomMaxMembers,
	})
	limiter := httpx.NewLimiter(cfg.JoinRatePerMinute, cfg.JoinRateBurst)
	// Der QR-Endpoint bekommt ein eigenes, großzügigeres Budget. Er braucht
	// einen gültigen Token, den der Aufrufer ohnehin schon hat, und darf
	// deshalb nicht mit den Beitrittsversuchen um dieselben Tokens
	// konkurrieren — sonst zeigt ein normaler Reload ein kaputtes Bild.
	qrLimiter := httpx.NewLimiter(cfg.JoinRatePerMinute*6, cfg.JoinRateBurst*4)

	done := make(chan struct{})
	defer close(done)
	go hub.Run(done)
	go limiter.Run(done)
	go qrLimiter.Run(done)

	wsHandler := ws.New(hub, signer, limiter, ws.Limits{
		MaxFileSize:     cfg.MaxFileSize,
		MaxChunkSize:    cfg.MaxChunkSize,
		MaxTextItemSize: cfg.MaxTextItemSize,
		MaxLiveTextSize: cfg.MaxLiveTextSize,
	}, cfg.PublicURL.Host, cfg.RoomURL, cfg.TrustProxy, log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("GET /ws", wsHandler)
	mux.Handle("GET /api/qr", qr.New(hub, qrLimiter, cfg.RoomURL, cfg.TrustProxy))
	oidcAuth.Mount(mux)
	mux.HandleFunc("GET /api/me", signer.MeHandler)
	mux.Handle("GET /api/avatar", avatar.New(signer, cfg.AvatarHosts, log))
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
