package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"crm/pkg/store"
	"crm/pkg/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// A team behind one NAT shares one budget, so session routes must not spend the
// budget that protects bcrypt, and the other way around.
func TestPasswordAndSessionRoutesHaveSeparateBudgets(t *testing.T) {
	h := authHandler(t, WithAuthRateLimit(2), WithSessionRateLimit(1_000_000))
	s := register(t, h, uniqueEmail(t)) // Spends one password token.

	wrong := map[string]string{"email": uniqueEmail(t), "password": "wrong password here"}
	post(t, h, "/auth/login", wrong)
	if rec := post(t, h, "/auth/login", wrong); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("login over the password budget = %d, want 429", rec.Code)
	}
	if rec := post(t, h, "/auth/register", wrong); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("register over the password budget = %d, want 429", rec.Code)
	}

	// The password budget is spent, and the session stays fully usable.
	for i := range 20 {
		if rec := me(h, "Bearer "+s.Token); rec.Code != http.StatusOK {
			t.Fatalf("me %d = %d, want 200", i, rec.Code)
		}
	}
	rec := refresh(t, h, s.RefreshToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh = %d, want 200", rec.Code)
	}
	next := decodeSession(t, rec)
	if rec := post(t, h, "/auth/logout", map[string]string{"refresh_token": next.RefreshToken}); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", rec.Code)
	}
}

func TestSessionRoutesAreStillLimited(t *testing.T) {
	h := authHandler(t, WithAuthRateLimit(1_000_000), WithSessionRateLimit(2))
	s := register(t, h, uniqueEmail(t))
	me(h, "Bearer "+s.Token)
	me(h, "Bearer "+s.Token)
	limited := me(h, "Bearer "+s.Token)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("me over the session budget = %d, want 429", limited.Code)
	}
	if limited.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
	// Spending the session budget leaves password routes alone.
	body := map[string]string{"email": uniqueEmail(t), "password": "wrong password here"}
	if rec := post(t, h, "/auth/login", body); rec.Code != http.StatusUnauthorized {
		t.Errorf("login = %d, want 401", rec.Code)
	}
}

// silentPool connects to a listener that accepts and never answers, so only a
// context deadline can end a query against it.
func silentPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	pool, err := pgxpool.New(context.Background(), fmt.Sprintf("postgres://u:p@%s/db?sslmode=disable", ln.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			conn.Close()
		}
	})
	return pool
}

// answersWithin fails the test if a request outlives budget. It leaves the
// handler goroutine behind on failure; the pool's cleanup unblocks it.
func answersWithin(t *testing.T, budget time.Duration, name string, h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rec, r)
	}()
	select {
	case <-done:
		return rec
	case <-time.After(budget):
		t.Errorf("%s still running after %s: no database deadline", name, budget)
		return nil
	}
}

func TestDatabaseCallsHaveDeadlines(t *testing.T) {
	manager := testManager(t, time.Minute)
	h := New(store.New(silentPool(t)), manager,
		WithPasswordCost(bcrypt.MinCost), WithDBTimeout(100*time.Millisecond),
		WithAuthRateLimit(1_000_000), WithSessionRateLimit(1_000_000)).Handler()
	token, err := manager.Generate("00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	meRequest := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	meRequest.Header.Set("Authorization", "Bearer "+token)

	tests := []struct {
		name string
		r    *http.Request
	}{
		{"login", httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"a@example.test","password":"long enough password"}`))},
		{"register", httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"email":"a@example.test","name":"A","password":"long enough password"}`))},
		{"me", meRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := answersWithin(t, 3*time.Second, tt.name, h, tt.r)
			if rec != nil && rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", rec.Code)
			}
		})
	}
}

// Login's lookup succeeds, and a held lock on the user row blocks the foreign
// key check of the session insert, so only that step can hit the deadline.
func TestSessionIssueHasDeadline(t *testing.T) {
	pool := testdb.AppPool(t)
	h := New(store.New(pool), testManager(t, time.Minute),
		WithPasswordCost(bcrypt.MinCost), WithDBTimeout(200*time.Millisecond),
		WithAuthRateLimit(1_000_000), WithSessionRateLimit(1_000_000)).Handler()
	email := uniqueEmail(t)
	s := register(t, h, email)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, s.User.ID); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"email": email, "password": testPassword})
	rec := answersWithin(t, 3*time.Second, "login", h, httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body)))
	if rec != nil && rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	tx.Rollback(ctx)
}

func TestSaturatedPasswordWorkAnswers503(t *testing.T) {
	a := New(store.New(testdb.AppPool(t)), testManager(t, time.Minute),
		WithPasswordCost(bcrypt.MinCost), WithPasswordConcurrency(1, 20*time.Millisecond),
		WithAuthRateLimit(1_000_000), WithSessionRateLimit(1_000_000))
	h := a.Handler()
	email := uniqueEmail(t)
	register(t, h, email)

	a.passwordSlots <- struct{}{} // Occupy the only slot.

	wrong := post(t, h, "/auth/login", map[string]string{"email": email, "password": "wrong password here"})
	unknown := post(t, h, "/auth/login", map[string]string{"email": uniqueEmail(t), "password": "wrong password here"})
	created := post(t, h, "/auth/register", map[string]string{"email": uniqueEmail(t), "name": "A", "password": testPassword})
	for name, rec := range map[string]*httptest.ResponseRecorder{"wrong password": wrong, "unknown email": unknown, "register": created} {
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s = %d, want 503", name, rec.Code)
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Errorf("%s lacks Retry-After", name)
		}
	}
	if wrong.Body.String() != unknown.Body.String() || fmt.Sprint(wrong.Header()) != fmt.Sprint(unknown.Header()) {
		t.Errorf("saturation tells accounts apart: %q vs %q", wrong.Body, unknown.Body)
	}

	<-a.passwordSlots
	if rec := post(t, h, "/auth/login", map[string]string{"email": email, "password": testPassword}); rec.Code != http.StatusOK {
		t.Errorf("login after release = %d, want 200", rec.Code)
	}
}

func TestPasswordWorkIsBounded(t *testing.T) {
	const slots = 2
	a := New(store.New(unreachablePool(t)), testManager(t, time.Minute),
		WithPasswordCost(bcrypt.MinCost), WithPasswordConcurrency(slots, time.Minute))

	var running, peak atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			ok := a.withPasswordSlot(context.Background(), func() {
				now := running.Add(1)
				for {
					seen := peak.Load()
					if now <= seen || peak.CompareAndSwap(seen, now) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				running.Add(-1)
			})
			if !ok {
				t.Error("withPasswordSlot refused inside its wait budget")
			}
		})
	}
	wg.Wait()
	if got := peak.Load(); got > slots {
		t.Errorf("%d password jobs ran at once, want at most %d", got, slots)
	}
	if got := len(a.passwordSlots); got != 0 {
		t.Errorf("%d slots still held after all jobs finished", got)
	}
}

func TestPasswordWaitEndsWithClientContext(t *testing.T) {
	a := New(store.New(unreachablePool(t)), testManager(t, time.Minute),
		WithPasswordCost(bcrypt.MinCost), WithPasswordConcurrency(1, time.Hour))
	a.passwordSlots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a.withPasswordSlot(ctx, func() { t.Error("ran without a slot") }) {
		t.Error("withPasswordSlot = true with a cancelled context and no slot")
	}
}

// PostgreSQL text cannot hold a NUL, so without this check a JSON-escaped one
// reaches the database and comes back as a server error.
func TestNULInAuthInputIsAClientError(t *testing.T) {
	h := authHandler(t)
	const nul = "\x00"
	tests := []struct {
		name string
		path string
		body map[string]string
	}{
		{"register email", "/auth/register", map[string]string{"email": "a" + nul + "@example.test", "name": "A", "password": testPassword}},
		{"register name", "/auth/register", map[string]string{"email": uniqueEmail(t), "name": "A" + nul, "password": testPassword}},
		{"register password", "/auth/register", map[string]string{"email": uniqueEmail(t), "name": "A", "password": testPassword + nul}},
		{"login email", "/auth/login", map[string]string{"email": "a" + nul + "@example.test", "password": testPassword}},
		{"login password", "/auth/login", map[string]string{"email": uniqueEmail(t), "password": testPassword + nul}},
		{"refresh token", "/auth/refresh", map[string]string{"refresh_token": "abc" + nul}},
		{"logout token", "/auth/logout", map[string]string{"refresh_token": "abc" + nul}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := post(t, h, tt.path, tt.body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
		})
	}
}
