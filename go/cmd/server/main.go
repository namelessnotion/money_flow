// Serves the Twirp APIs backed by the Postgres event store.
//
//	DATABASE_URL=postgres://... LISTEN_ADDR=:8080 go run ./cmd/server
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	holderpb "github.com/namelessnotion/money_flow/go/gen/proto/holder/v1"
	walletpb "github.com/namelessnotion/money_flow/go/gen/proto/wallet/v1"
	"github.com/namelessnotion/money_flow/go/internal/eventstore"
	"github.com/namelessnotion/money_flow/go/internal/holder"
	"github.com/namelessnotion/money_flow/go/internal/wallet"
)

const (
	defaultDatabaseURL = "postgres://money_flow:money_flow@localhost:5432/money_flow_dev?sslmode=disable"
	defaultListenAddr  = ":8080"
	shutdownGrace      = 10 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, env("DATABASE_URL", defaultDatabaseURL))
	if err != nil {
		log.Fatalf("server: pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("server: ping: %v", err)
	}

	addr := env("LISTEN_ADDR", defaultListenAddr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           newMux(eventstore.NewPostgresStore(pool), pool),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("server: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("server: shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server: shutdown: %v", err)
	}
}

// pinger is the part of the pool the health check needs, so tests can supply
// something that isn't a real database.
type pinger interface {
	Ping(context.Context) error
}

// newMux wires the services onto their generated Twirp paths. Each service
// mounts at its own PathPrefix() — "/twirp/<package>.<Service>/" — which is
// what clients must post to; a client pointed at the bare host will 404.
func newMux(store eventstore.Store, health pinger) *http.ServeMux {
	mux := http.NewServeMux()

	holderServer := holderpb.NewHolderServiceServer(holder.NewServer(store))
	walletServer := walletpb.NewWalletServiceServer(wallet.NewServer(store))
	mux.Handle(holderServer.PathPrefix(), holderServer)
	mux.Handle(walletServer.PathPrefix(), walletServer)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := health.Ping(r.Context()); err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	return mux
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
