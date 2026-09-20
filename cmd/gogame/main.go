// Command gogame serves the Alexander the Great card-adventure game on a
// local HTTP server. All content, templates, and assets are embedded.
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

	"goGame/internal/content"
	"goGame/internal/web"

	assets "goGame"
)

// Stale sessions lose their saves after a month of inactivity; the store
// is swept for them at startup and once a day thereafter.
const (
	saveMaxAge = 30 * 24 * time.Hour
	sweepEvery = 24 * time.Hour
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	saves := flag.String("saves", "saves", "save directory")
	flag.Parse()

	lib, err := content.Load(assets.FS())
	if err != nil {
		log.Fatal(err)
	}
	store := web.NewStore(lib, *saves)
	sweepStale(store)
	go sweepDaily(store)
	srv := web.New(store)
	srv.Addr = *addr

	// Graceful shutdown: finish in-flight requests, then exit.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Println("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("gogame listening on http://%s", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func sweepStale(store *web.Store) {
	if n := store.Sweep(saveMaxAge); n > 0 {
		log.Printf("swept %d stale save(s)", n)
	}
}

func sweepDaily(store *web.Store) {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()
	for range ticker.C {
		sweepStale(store)
	}
}
