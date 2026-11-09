// Package app holds the HTTP routes and handlers.
package app

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/netip"
	"runtime"
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
	// A team can share one client address, and every signed-in screen spends this
	// budget, so it is far larger than the budget that protects bcrypt.
	defaultSessionRateLimit = 600
	defaultDBTimeout        = 5 * time.Second
	defaultPasswordWait     = 3 * time.Second
)

type App struct {
	store             *store.Store
	tokens            *auth.Manager
	passwords         *auth.Passwords
	passwordLimit     *ipLimiter
	sessionLimit      *ipLimiter
	workspaceLimit    *ipLimiter
	memberLookupLimit *ipLimiter
	healthTimeout     time.Duration
	passwordCost      int
	refreshTTL        time.Duration
	refreshGrace      time.Duration
	rateLimit         int
	corsOrigins       []string
	trust             proxyTrust

	sessionRateLimit      int
	memberLookupRateLimit int
	dbTimeout             time.Duration
	passwordSlots         chan struct{}
	passwordWait          time.Duration
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

// WithAuthRateLimit sets the per-client request budget per minute on password
// routes.
func WithAuthRateLimit(perMinute int) Option {
	return func(a *App) { a.rateLimit = perMinute }
}

// WithSessionRateLimit sets the per-client request budget per minute on routes
// that need no password work.
func WithSessionRateLimit(perMinute int) Option {
	return func(a *App) { a.sessionRateLimit = perMinute }
}

// WithMemberLookupRateLimit sets the per-client budget per minute for adding a
// member, which reveals whether an email has an account. It defaults to the
// password route budget.
func WithMemberLookupRateLimit(perMinute int) Option {
	return func(a *App) { a.memberLookupRateLimit = perMinute }
}

// WithDBTimeout bounds each database step of a request.
func WithDBTimeout(d time.Duration) Option {
	return func(a *App) { a.dbTimeout = d }
}

// WithPasswordConcurrency limits how many bcrypt operations run at once and how
// long a request waits for a slot before it is refused.
func WithPasswordConcurrency(slots int, wait time.Duration) Option {
	return func(a *App) {
		a.passwordSlots = make(chan struct{}, slots)
		a.passwordWait = wait
	}
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

		sessionRateLimit: defaultSessionRateLimit,
		dbTimeout:        defaultDBTimeout,
		// bcrypt is pure CPU, so more jobs than cores only adds latency.
		passwordSlots: make(chan struct{}, max(2, runtime.GOMAXPROCS(0))),
		passwordWait:  defaultPasswordWait,
	}
	for _, o := range opts {
		o(app)
	}
	passwords, err := auth.NewPasswords(app.passwordCost)
	if err != nil {
		panic(err)
	}
	app.passwords = passwords
	app.passwordLimit = newIPLimiter(app.rateLimit, limiterMaxEntries, time.Now)
	app.sessionLimit = newIPLimiter(app.sessionRateLimit, limiterMaxEntries, time.Now)
	// Same size as the session budget but a separate bucket, so a busy screen
	// cannot starve the token refresh that keeps it signed in.
	app.workspaceLimit = newIPLimiter(app.sessionRateLimit, limiterMaxEntries, time.Now)
	if app.memberLookupRateLimit == 0 {
		app.memberLookupRateLimit = app.rateLimit
	}
	app.memberLookupLimit = newIPLimiter(app.memberLookupRateLimit, limiterMaxEntries, time.Now)
	return app
}

func (a *App) Handler() http.Handler {
	r := chi.NewRouter()
	// Outermost first: every response carries security and CORS headers, and
	// preflights are answered before they can spend the rate limit budget.
	r.Use(securityHeaders(a.trust), cors(a.corsOrigins))

	r.Get("/health", a.health)

	r.Route("/auth", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(rateLimit(a.passwordLimit, a.trust))
			r.Post("/register", a.register)
			r.Post("/login", a.login)
		})
		r.Group(func(r chi.Router) {
			r.Use(rateLimit(a.sessionLimit, a.trust))
			r.Post("/refresh", a.refresh)
			r.Post("/logout", a.logout)
			r.With(a.requireAuth).Get("/me", a.me)
		})
	})

	r.Route("/workspaces", func(r chi.Router) {
		r.Use(rateLimit(a.workspaceLimit, a.trust), a.requireAuth)
		r.Post("/", a.createWorkspace)
		r.Get("/", a.listWorkspaces)
		r.Route("/{"+workspaceParam+"}", func(r chi.Router) {
			r.Use(a.requireWorkspace)
			r.Get("/", a.getWorkspace)
			r.Patch("/", a.renameWorkspace)
			r.Delete("/", a.deleteWorkspace)
			r.Post("/leave", a.leaveWorkspace)
			r.Get("/members", a.listMembers)
			r.With(rateLimit(a.memberLookupLimit, a.trust)).Post("/members", a.addMember)
			r.Patch("/members/{"+memberParam+"}", a.changeMemberRole)
			r.Delete("/members/{"+memberParam+"}", a.removeMember)
			r.Route("/companies", func(r chi.Router) {
				r.Get("/", a.listCompanies)
				r.Post("/", a.createCompany)
				r.Get("/{"+companyParam+"}", a.getCompany)
				r.Patch("/{"+companyParam+"}", a.updateCompany)
				r.Delete("/{"+companyParam+"}", a.deleteCompany)
			})
			r.Route("/contacts", func(r chi.Router) {
				r.Get("/", a.listContacts)
				r.Post("/", a.createContact)
				r.Get("/{"+contactParam+"}", a.getContact)
				r.Patch("/{"+contactParam+"}", a.updateContact)
				r.Delete("/{"+contactParam+"}", a.deleteContact)
			})
			r.Route("/pipelines", func(r chi.Router) {
				r.Get("/", a.listPipelines)
				r.Post("/", a.createPipeline)
				r.Route("/{"+pipelineParam+"}", func(r chi.Router) {
					r.Get("/", a.getPipeline)
					r.Patch("/", a.renamePipeline)
					r.Delete("/", a.deletePipeline)
					r.Post("/stages", a.createStage)
					r.Put("/stages/order", a.reorderStages)
					r.Patch("/stages/{"+stageParam+"}", a.updateStage)
					r.Delete("/stages/{"+stageParam+"}", a.deleteStage)
				})
			})
			r.Route("/deals", func(r chi.Router) {
				r.Get("/", a.listDeals)
				r.Post("/", a.createDeal)
				r.Get("/{"+dealParam+"}", a.getDeal)
				r.Patch("/{"+dealParam+"}", a.updateDeal)
				r.Delete("/{"+dealParam+"}", a.deleteDeal)
				r.Post("/{"+dealParam+"}/move", a.moveDeal)
			})
		})
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
