// Package app holds the HTTP routes and handlers.
package app

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/netip"
	"time"

	"crm/pkg/auth"
	"crm/pkg/store"

	"github.com/go-chi/chi/v5"
)

const (
	defaultHealthTimeout = 2 * time.Second
	defaultRefreshTTL    = 30 * 24 * time.Hour
	defaultRefreshGrace  = 10 * time.Second
	defaultAuthRateLimit = 30
)

type App struct {
	store         *store.Store
	tokens        *auth.Manager
	passwords     *auth.Passwords
	limiter       *ipLimiter
	healthTimeout time.Duration
	passwordCost  int
	refreshTTL    time.Duration
	refreshGrace  time.Duration
	rateLimit     int
	corsOrigins   []string
	trust         proxyTrust
}

type Option func(*App)

// WithHealthTimeout bounds how long GET /health waits for the database.
func WithHealthTimeout(d time.Duration) Option {
	return func(a *App) {
		a.healthTimeout = d
	}
}

// WithPasswordCost sets the bcrypt work factor.
func WithPasswordCost(cost int) Option {
	return func(a *App) { a.passwordCost = cost }
}

// WithRefreshTTL sets how long a refresh token stays valid.
func WithRefreshTTL(d time.Duration) Option {
	return func(a *App) { a.refreshTTL = d }
}

// WithRefreshGrace sets how long after rotation a spent refresh token may be
// retried. Zero disables retries.
func WithRefreshGrace(d time.Duration) Option {
	return func(a *App) { a.refreshGrace = d }
}

// WithAuthRateLimit sets the per-client request budget per minute on auth routes.
func WithAuthRateLimit(perMinute int) Option {
	return func(a *App) { a.rateLimit = perMinute }
}

// WithCORSOrigins sets the browser origins allowed to call the API.
func WithCORSOrigins(origins []string) Option {
	return func(a *App) { a.corsOrigins = origins }
}

// WithTrustedProxies sets the networks whose forwarding headers are believed.
func WithTrustedProxies(prefixes []netip.Prefix) Option {
	return func(a *App) { a.trust = prefixes }
}

// New builds the app. It panics on an invalid password cost, which is fixed in
// code rather than read from input.
func New(s *store.Store, tokens *auth.Manager, opts ...Option) *App {
	app := &App{
		store:         s,
		tokens:        tokens,
		healthTimeout: defaultHealthTimeout,
		passwordCost:  auth.DefaultCost,
		refreshTTL:    defaultRefreshTTL,
		refreshGrace:  defaultRefreshGrace,
		rateLimit:     defaultAuthRateLimit,
	}
	for _, o := range opts {
		o(app)
	}
	passwords, err := auth.NewPasswords(app.passwordCost)
	if err != nil {
		panic(err)
	}
	app.passwords = passwords
	app.limiter = newIPLimiter(app.rateLimit, limiterMaxEntries, time.Now)
	return app
}

func (a *App) Handler() http.Handler {
	r := chi.NewRouter()
	// Outermost first: every response carries security and CORS headers, and
	// preflights are answered before they can spend the rate limit budget.
	r.Use(securityHeaders(a.trust), cors(a.corsOrigins))

	r.Get("/health", a.health)

	r.Route("/auth", func(r chi.Router) {
		r.Use(rateLimit(a.limiter, a.trust))
		r.Post("/register", a.register)
		r.Post("/login", a.login)
		r.Post("/refresh", a.refresh)
		r.Post("/logout", a.logout)
		r.With(a.requireAuth).Get("/me", a.me)
	})

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
