// Package server wires configuration, database, and routes into a running HTTP
// server.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"crm/pkg/app"
	"crm/pkg/config"
	"crm/pkg/migrate"
	"crm/pkg/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	startupTimeout  = 30 * time.Second
	shutdownTimeout = 10 * time.Second
)

type Server struct {
	http *http.Server
	pool *pgxpool.Pool
}

// New connects to the database and applies pending migrations. The caller
// owns the result and releases it by calling Run.
func New(ctx context.Context, cfg config.Config) (*Server, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("configure database pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := migrate.Apply(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return &Server{
		http: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           app.New(store.New(pool)).Handler(),
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		pool: pool,
	}, nil
}

// Run serves until ctx is cancelled, drains in-flight requests, and closes the
// database pool.
func (s *Server) Run(ctx context.Context) error {
	defer s.pool.Close()

	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return err
	}
	log.Printf("listening on %s", ln.Addr())
	return serve(ctx, s.http, ln)
}

func serve(ctx context.Context, srv *http.Server, ln net.Listener) error {
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}

	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return errors.Join(fmt.Errorf("graceful shutdown: %w", err), srv.Close())
	}
	return nil
}
