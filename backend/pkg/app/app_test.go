package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"crm/pkg/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getHealth(t *testing.T, s *store.Store) *httptest.ResponseRecorder {
	t.Helper()
	h := New(s, WithHealthTimeout(2*time.Second)).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	return rec
}

// Connects lazily, so no database is needed: nothing listens on port 1.
func TestHealthReturns503WhenDatabaseUnreachable(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://crm:secret-password@127.0.0.1:1/crm?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	rec := getHealth(t, store.New(pool))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body, err)
	}
	if body.Error != "database unavailable" {
		t.Errorf("error = %q, want %q", body.Error, "database unavailable")
	}
	// Driver errors name the host and user; they belong in logs, not responses.
	if strings.Contains(rec.Body.String(), "127.0.0.1") {
		t.Errorf("response leaks connection details: %s", rec.Body)
	}
}

func TestHealthReturns200WhenDatabaseReachable(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	rec := getHealth(t, store.New(pool))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var body struct {
		Data struct {
			Status   string `json:"status"`
			Database string `json:"database"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body, err)
	}
	if body.Data.Status != "ok" || body.Data.Database != "ok" {
		t.Errorf("body = %s, want status and database ok", rec.Body)
	}
}
