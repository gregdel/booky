package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gregdel/booky/internal/caldav"
	"github.com/gregdel/booky/internal/config"
	"github.com/gregdel/booky/internal/httpd"
	"github.com/gregdel/booky/web"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverReadTimeout       = 10 * time.Second
	serverWriteTimeout      = 45 * time.Second
	serverIdleTimeout       = 60 * time.Second
	serverMaxHeaderBytes    = 32 << 10
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Printf("failed to load config: %v", err)
		os.Exit(1)
	}

	store, err := caldav.New(cfg.CalDAV, nil)
	if err != nil {
		log.Printf("failed to create CalDAV client: %v", err)
		os.Exit(1)
	}

	handler := httpd.New(store, web.Files, cfg.PublicPath, cfg.AppTitle)
	server := newHTTPServer(cfg.ListenAddr, handler)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("booky listening on %s", cfg.ListenAddr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown failed: %v", err)
			os.Exit(1)
		}
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
		MaxHeaderBytes:    serverMaxHeaderBytes,
	}
}
