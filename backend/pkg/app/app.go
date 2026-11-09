// Package app holds the HTTP routes and handlers.
package app

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"crm/pkg/store"

	"github.com/go-chi/chi/v5"
)

const defaultHealthTimeout = 2 * time.Second

type App struct {
	store         *store.Store
	healthTimeout time.Duration
}

type Option func(*App)

// WithHealthTimeout bounds how long GET /health waits for the database.
func WithHealthTimeout(d time.Duration) Option {
	return func(a *App) {
		a.healthTimeout = d
	}
}

func New(s *store.Store, opts ...Option) *App {
	app := &App{
		store:         s,
		healthTimeout: defaultHealthTimeout,
	}
	for _, o := range opts {
		o(app)
	}
	return app
}

func (a *App) Handler() http.Handler {
	r := chi.NewRouter()

	r.Get("/health", a.health)

	return r
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.healthTimeout)
	defer cancel()

	if err := a.store.Ping(ctx); err != nil {
		log.Printf("health: database unreachable: %v", err)
		errResp(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	dataResp(w, http.StatusOK, map[string]string{"status": "ok", "database": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func dataResp(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, map[string]any{"data": v})
}

func errResp(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
