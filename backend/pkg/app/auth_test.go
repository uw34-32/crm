package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"crm/pkg/auth"
	"crm/pkg/store"
	"crm/pkg/testdb"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	testJWTSecret = "app-test-secret-that-is-long-enough-for-hs256"
	testPassword  = "correct horse battery"
)

func testManager(t *testing.T, ttl time.Duration) *auth.Manager {
	t.Helper()
	m, err := auth.NewManager(testJWTSecret, ttl)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// authHandler builds the app against the database as the app role. Tests get a
// generous rate limit and a cheap bcrypt cost unless opts override them.
func authHandler(t *testing.T, opts ...Option) http.Handler {
	t.Helper()
	base := []Option{WithPasswordCost(bcrypt.MinCost), WithAuthRateLimit(1_000_000), WithSessionRateLimit(1_000_000), WithRefreshGrace(0)}
	return New(store.New(testdb.AppPool(t)), testManager(t, time.Minute), append(base, opts...)...).Handler()
}

func uniqueEmail(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "user-" + hex.EncodeToString(b) + "@example.test"
}

func post(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b)))
	return rec
}

func me(h http.Handler, authorization string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

type session struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	User         struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"user"`
}

func decodeSession(t *testing.T, rec *httptest.ResponseRecorder) session {
	t.Helper()
	var body struct {
		Data session `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Data.Token == "" || body.Data.RefreshToken == "" {
		t.Fatalf("response lacks tokens (status %d)", rec.Code)
	}
	return body.Data
}

func register(t *testing.T, h http.Handler, email string) session {
	t.Helper()
	rec := post(t, h, "/auth/register", map[string]string{"email": email, "name": "Test User", "password": testPassword})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d: %s", rec.Code, rec.Body)
	}
	return decodeSession(t, rec)
}

func refresh(t *testing.T, h http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, h, "/auth/refresh", map[string]string{"refresh_token": token})
}

func TestAuthFullFlow(t *testing.T) {
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })

	h := authHandler(t)
	email := uniqueEmail(t)

	registered := register(t, h, email)
	if registered.User.Email != email || registered.User.Name != "Test User" || registered.User.ID == "" {
		t.Errorf("registered user = %+v", registered.User)
	}

	rec := post(t, h, "/auth/login", map[string]string{"email": strings.ToUpper(email), "password": testPassword})
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", rec.Code, rec.Body)
	}
	loggedIn := decodeSession(t, rec)
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "$2a$") {
		t.Error("login response exposes password material")
	}

	rec = me(h, "Bearer "+loggedIn.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d: %s", rec.Code, rec.Body)
	}
	var current struct {
		Data struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.Data.ID != registered.User.ID || current.Data.Email != email {
		t.Errorf("me = %s", rec.Body)
	}

	rec = refresh(t, h, loggedIn.RefreshToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh = %d: %s", rec.Code, rec.Body)
	}
	refreshed := decodeSession(t, rec)
	if refreshed.RefreshToken == loggedIn.RefreshToken {
		t.Error("refresh did not rotate the token")
	}
	if got := me(h, "Bearer "+refreshed.Token).Code; got != http.StatusOK {
		t.Errorf("me with the refreshed access token = %d", got)
	}

	rec = post(t, h, "/auth/logout", map[string]string{"refresh_token": refreshed.RefreshToken})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d: %s", rec.Code, rec.Body)
	}
	if got := refresh(t, h, refreshed.RefreshToken).Code; got != http.StatusUnauthorized {
		t.Errorf("refresh after logout = %d, want 401", got)
	}
	// Logging out one session leaves the other login alone.
	if got := refresh(t, h, registered.RefreshToken).Code; got != http.StatusOK {
		t.Errorf("other session after logout = %d, want 200", got)
	}

	for _, secret := range []string{testPassword, registered.Token, loggedIn.RefreshToken, refreshed.RefreshToken, "$2a$"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("logs contain secret material %.8q", secret)
		}
	}
}

func TestRegisterValidation(t *testing.T) {
	h := authHandler(t)
	long := strings.Repeat("x", auth.MaxPasswordBytes+1)
	tests := []struct {
		name string
		body map[string]string
	}{
		{"missing email", map[string]string{"name": "A", "password": testPassword}},
		{"malformed email", map[string]string{"email": "nope", "name": "A", "password": testPassword}},
		{"email with display name", map[string]string{"email": "A <a@example.test>", "name": "A", "password": testPassword}},
		{"oversized email", map[string]string{"email": strings.Repeat("a", 250) + "@example.test", "name": "A", "password": testPassword}},
		{"missing name", map[string]string{"email": uniqueEmail(t), "password": testPassword}},
		{"blank name", map[string]string{"email": uniqueEmail(t), "name": "   ", "password": testPassword}},
		{"oversized name", map[string]string{"email": uniqueEmail(t), "name": strings.Repeat("n", 201), "password": testPassword}},
		{"short password", map[string]string{"email": uniqueEmail(t), "name": "A", "password": "short"}},
		{"password bcrypt would truncate", map[string]string{"email": uniqueEmail(t), "name": "A", "password": long}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := post(t, h, "/auth/register", tt.body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestMalformedBodies(t *testing.T) {
	h := authHandler(t)
	huge := `{"email":"` + strings.Repeat("a", 2<<20) + `"}`
	for _, path := range []string{"/auth/register", "/auth/login", "/auth/refresh", "/auth/logout"} {
		for _, body := range []string{"", "not json", `{"email":1}`, `{} {}`, huge} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("POST %s with %.20q = %d, want 400", path, body, rec.Code)
			}
		}
	}
}

func TestRegisterDuplicateEmailConflicts(t *testing.T) {
	h := authHandler(t)
	email := uniqueEmail(t)
	register(t, h, email)
	rec := post(t, h, "/auth/register", map[string]string{"email": strings.ToUpper(email), "name": "B", "password": testPassword})
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	h := authHandler(t)
	email := uniqueEmail(t)
	register(t, h, email)

	wrong := post(t, h, "/auth/login", map[string]string{"email": email, "password": "wrong password here"})
	unknown := post(t, h, "/auth/login", map[string]string{"email": uniqueEmail(t), "password": "wrong password here"})
	tooLong := post(t, h, "/auth/login", map[string]string{"email": email, "password": strings.Repeat("x", 500)})

	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", wrong.Code)
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{"unknown email": unknown, "oversized password": tooLong} {
		if rec.Code != wrong.Code {
			t.Errorf("%s status = %d, wrong password = %d", name, rec.Code, wrong.Code)
		}
		if rec.Body.String() != wrong.Body.String() {
			t.Errorf("%s body = %q, wrong password = %q", name, rec.Body, wrong.Body)
		}
		if fmt.Sprint(rec.Header()) != fmt.Sprint(wrong.Header()) {
			t.Errorf("%s headers = %v, wrong password = %v", name, rec.Header(), wrong.Header())
		}
	}
}

// Skipping bcrypt for unknown emails would make them measurably faster. The
// minimum of several samples discards scheduling noise.
func TestLoginUnknownEmailSpendsBcryptTime(t *testing.T) {
	h := authHandler(t, WithPasswordCost(10))
	email := uniqueEmail(t)
	register(t, h, email)

	fastest := func(email string) time.Duration {
		best := time.Hour
		for range 5 {
			start := time.Now()
			post(t, h, "/auth/login", map[string]string{"email": email, "password": "wrong password here"})
			best = min(best, time.Since(start))
		}
		return best
	}
	known, unknown := fastest(email), fastest(uniqueEmail(t))
	if unknown < known/2 {
		t.Errorf("unknown email took %v, known email %v: bcrypt was skipped", unknown, known)
	}
}

func TestRefreshReuseRevokesFamily(t *testing.T) {
	h := authHandler(t)
	s := register(t, h, uniqueEmail(t))

	rec := refresh(t, h, s.RefreshToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("first refresh = %d: %s", rec.Code, rec.Body)
	}
	successor := decodeSession(t, rec)

	if got := refresh(t, h, s.RefreshToken).Code; got != http.StatusUnauthorized {
		t.Errorf("reuse = %d, want 401", got)
	}
	if got := refresh(t, h, successor.RefreshToken).Code; got != http.StatusUnauthorized {
		t.Errorf("successor after reuse = %d, want 401: family not revoked", got)
	}
}

func TestRefreshRejectsExpiredToken(t *testing.T) {
	h := authHandler(t, WithRefreshTTL(-time.Minute))
	s := register(t, h, uniqueEmail(t))
	rec := refresh(t, h, s.RefreshToken)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401: %s", rec.Code, rec.Body)
	}
}

func TestRefreshInvalidTokenResponsesMatch(t *testing.T) {
	h := authHandler(t)
	s := register(t, h, uniqueEmail(t))
	refresh(t, h, s.RefreshToken)

	reused := refresh(t, h, s.RefreshToken)
	unknown := refresh(t, h, "never-issued")
	if reused.Code != unknown.Code || reused.Body.String() != unknown.Body.String() {
		t.Errorf("reused = %d %q, unknown = %d %q", reused.Code, reused.Body, unknown.Code, unknown.Body)
	}
}

func TestConcurrentRefreshOutsideGraceHasOneWinner(t *testing.T) {
	h := authHandler(t)
	s := register(t, h, uniqueEmail(t))

	const racers = 8
	codes := make([]int, racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() {
			<-start
			codes[i] = refresh(t, h, s.RefreshToken).Code
		})
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			wins++
		case http.StatusUnauthorized:
		default:
			t.Errorf("unexpected status %d", code)
		}
	}
	if wins != 1 {
		t.Errorf("%d refreshes succeeded, want exactly 1: %v", wins, codes)
	}
}

func TestMeRejectsBadCredentials(t *testing.T) {
	h := authHandler(t)
	s := register(t, h, uniqueEmail(t))

	sign := func(method jwt.SigningMethod, key any, ttl time.Duration) string {
		t.Helper()
		token, err := jwt.NewWithClaims(method, jwt.RegisteredClaims{
			Issuer:    "crm",
			Subject:   s.User.ID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		}).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	expired, err := testManager(t, -time.Minute).Generate(s.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	nobody, err := testManager(t, time.Minute).Generate("00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct{ name, header string }{
		{"no header", ""},
		{"wrong scheme", "Basic " + s.Token},
		{"no token", "Bearer "},
		{"refresh token as access token", "Bearer " + s.RefreshToken},
		{"garbage", "Bearer not-a-token"},
		{"expired", "Bearer " + expired},
		{"wrong algorithm HS512", "Bearer " + sign(jwt.SigningMethodHS512, []byte(testJWTSecret), time.Hour)},
		{"alg none", "Bearer " + sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, time.Hour)},
		{"wrong secret", "Bearer " + sign(jwt.SigningMethodHS256, []byte(testJWTSecret+"x"), time.Hour)},
		{"valid signature for a missing user", "Bearer " + nobody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := me(h, tt.header)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401: %s", rec.Code, rec.Body)
			}
		})
	}
	if got := me(h, "bearer "+s.Token).Code; got != http.StatusOK {
		t.Errorf("the auth scheme is case-insensitive, got %d", got)
	}
}

// The handler behind the middleware must never run without a verified token,
// whatever it would do with an empty identity.
func TestRequireAuthStopsUnverifiedRequests(t *testing.T) {
	manager := testManager(t, time.Minute)
	a := New(store.New(unreachablePool(t)), manager, WithPasswordCost(bcrypt.MinCost))
	called, reached := false, ""
	h := a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		reached, _ = r.Context().Value(contextKey{}).(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	send := func(header string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	for _, header := range []string{"", "Bearer", "Bearer not-a-token", "Basic abc"} {
		if got := send(header); got != http.StatusUnauthorized {
			t.Errorf("%q = %d, want 401", header, got)
		}
		if called {
			t.Fatalf("%q reached the handler", header)
		}
	}

	token, err := manager.Generate("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := send("Bearer " + token); got != http.StatusNoContent || reached != "user-1" {
		t.Errorf("valid token = %d with identity %q", got, reached)
	}
}

// A store failure must surface as an error, never as an authenticated response.
func TestAuthFailsClosedWhenDatabaseUnavailable(t *testing.T) {
	pool := unreachablePool(t)
	manager := testManager(t, time.Minute)
	h := New(store.New(pool), manager, WithPasswordCost(bcrypt.MinCost)).Handler()
	token, err := manager.Generate("00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/auth/me", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("me = %d, want 500", rec.Code)
	}
	login := post(t, h, "/auth/login", map[string]string{"email": "a@example.test", "password": testPassword})
	if login.Code != http.StatusInternalServerError {
		t.Errorf("login = %d, want 500", login.Code)
	}
}

func TestAuthRoutesAreRateLimited(t *testing.T) {
	h := authHandler(t, WithAuthRateLimit(3))
	body := map[string]string{"email": uniqueEmail(t), "password": "wrong password here"}
	for i := range 3 {
		if rec := post(t, h, "/auth/login", body); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, rec.Code)
		}
	}
	rec := post(t, h, "/auth/login", body)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt over the limit = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
	// The health route stays out of the limiter, so monitoring keeps working.
	health := httptest.NewRecorder()
	h.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code == http.StatusTooManyRequests {
		t.Error("the health route was rate limited")
	}
}

func TestMiddlewareIsWiredIntoHandler(t *testing.T) {
	h := authHandler(t, WithCORSOrigins([]string{"http://localhost:5173"}), WithAuthRateLimit(1))

	r := httptest.NewRequest(http.MethodOptions, "/auth/login", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	r.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("preflight = %d, Allow-Origin %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security headers missing from the preflight")
	}

	// Preflights do not spend the budget, and a 429 stays readable by the browser.
	send := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"a@example.test","password":"nope-nope-nope"}`))
		r.Header.Set("Origin", "http://localhost:5173")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	if got := send().Code; got == http.StatusTooManyRequests {
		t.Error("the preflight spent the rate limit budget")
	}
	limited := send()
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", limited.Code)
	}
	if limited.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("the 429 lacks CORS headers, so a browser client could not read it")
	}
}
