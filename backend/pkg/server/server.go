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
	"crm/pkg/auth"
	"crm/pkg/config"
	"crm/pkg/migrate"
	"crm/pkg/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

const shutdownTimeout = 10 * time.Second

// startupTimeout bounds the initial connection check. Migrations have their
// own deadline, config.Config.MigrationTimeout. It is a variable so tests can
// shorten it.
var startupTimeout = 30 * time.Second

type Server struct {
	http *http.Server
	pool *pgxpool.Pool
}

// New verifies the app role, applies pending migrations as the owner, verifies
// Row-Level Security on workspace-scoped tables, and prepares the server. The caller owns the result and releases it by calling
// Run.
func New(ctx context.Context, cfg config.Config) (*Server, error) {
	tokens, err := auth.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("configure database pool: %w", err)
	}

	if err := startup(ctx, cfg, pool); err != nil {
		pool.Close()
		return nil, err
	}

	return &Server{
		http: &http.Server{
			Addr: ":" + cfg.Port,
			Handler: app.New(store.New(pool), tokens,
				app.WithRefreshTTL(cfg.RefreshTokenTTL),
				app.WithRefreshGrace(cfg.RefreshGrace),
				app.WithAuthRateLimit(cfg.AuthRateLimitPerMinute),
				app.WithSessionRateLimit(cfg.SessionRateLimitPerMinute),
				app.WithCORSOrigins(cfg.CORSOrigins),
				app.WithTrustedProxies(cfg.TrustedProxies),
			).Handler(),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		pool: pool,
	}, nil
}

func startup(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	// Checked first, so a server that would refuse to boot changes nothing.
	connectCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := store.New(pool).CheckAppRole(connectCtx); err != nil {
		return fmt.Errorf("check database role: %w", err)
	}

	migrateCtx, cancel := context.WithTimeout(ctx, cfg.MigrationTimeout)
	defer cancel()
	owner, err := pgxpool.New(migrateCtx, cfg.MigrationDatabaseURL)
	if err != nil {
		return fmt.Errorf("configure migration connection: %w", err)
	}
	defer owner.Close()
	if err := migrate.Apply(migrateCtx, owner); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	// After migrations, because they are what creates or fixes the tables. A
	// table with a workspace_id that lacks its policies stops the server.
	checkCtx, cancelCheck := context.WithTimeout(ctx, startupTimeout)
	defer cancelCheck()
	if _, err := store.New(pool).CheckTenantTables(checkCtx); err != nil {
		return fmt.Errorf("check tenant tables: %w", err)
	}
	return nil
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
