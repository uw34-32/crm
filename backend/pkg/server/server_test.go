package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"crm/pkg/auth"
	"crm/pkg/config"
	"crm/pkg/store"
	"crm/pkg/testdb"

	"github.com/jackc/pgx/v5"
)

func TestServeDrainsInFlightRequestsOnCancel(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		io.WriteString(w, "finished")
	})}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serve(ctx, srv, ln) }()

	type result struct {
		body string
		err  error
	}
	response := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			response <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		response <- result{string(b), err}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request never reached the handler")
	}

	cancel()
	select {
	case err := <-served:
		t.Fatalf("serve returned with a request still in flight: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	select {
	case r := <-response:
		if r.err != nil || r.body != "finished" {
			t.Errorf("in-flight response = %q, %v", r.body, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight request never finished")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Errorf("serve = %v, want nil after a clean shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after shutdown")
	}
}

func TestServeReturnsListenerErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()

	if err := serve(context.Background(), &http.Server{}, ln); err == nil {
		t.Fatal("expected an error from a closed listener")
	}
}

func TestNewFailsWhenDatabaseUnreachable(t *testing.T) {
	_, err := New(context.Background(), config.Config{
		DatabaseURL:          "postgres://crm:x@127.0.0.1:1/crm?sslmode=disable&connect_timeout=1",
		MigrationDatabaseURL: "postgres://crm:x@127.0.0.1:1/crm?sslmode=disable&connect_timeout=1",
		Port:                 "0",
		MigrationTimeout:     time.Minute,
		JWTSecret:            testJWTSecret,
	})
	if err == nil {
		t.Fatal("expected an error when the database is unreachable")
	}
}

const testJWTSecret = "server-test-secret-long-enough-for-hs256"

// migrationLockKey mirrors the runner's advisory lock key, which must stay
// stable across releases. Holding it makes New wait inside migrate.Apply.
const migrationLockKey int64 = 7_260_001

// holdMigrationLock takes the runner's lock and returns a function that
// releases it. Release is safe to call more than once.
func holdMigrationLock(t *testing.T, url string) (release func()) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		t.Fatal(err)
	}
	release = sync.OnceFunc(func() { conn.Close(ctx) })
	t.Cleanup(release)
	return release
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

func TestNewAndRunServeHealthAgainstRealDatabase(t *testing.T) {
	appURL, ownerURL := testdb.URLs(t)
	port := freePort(t)

	srv, err := New(context.Background(), config.Config{
		DatabaseURL:          appURL,
		MigrationDatabaseURL: ownerURL,
		Port:                 port,
		MigrationTimeout:     time.Minute,
		JWTSecret:            testJWTSecret,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ran := make(chan error, 1)
	go func() { ran <- srv.Run(ctx) }()

	var resp *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err = http.Get("http://127.0.0.1:" + port + "/health")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never answered: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET /health = %d, want 200: %s", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-ran:
		if err != nil {
			t.Errorf("Run = %v, want nil after a clean shutdown", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestNewAbortsWhenMigrationDeadlineExpires(t *testing.T) {
	appURL, ownerURL := testdb.URLs(t)
	holdMigrationLock(t, ownerURL)

	start := time.Now()
	_, err := New(context.Background(), config.Config{
		DatabaseURL:          appURL,
		MigrationDatabaseURL: ownerURL,
		Port:                 "0",
		MigrationTimeout:     300 * time.Millisecond,
		JWTSecret:            testJWTSecret,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("New = %v, want a deadline error", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("New took %v, want it bounded by MigrationTimeout", elapsed)
	}
}

// A slow migration must outlive the connection-check deadline: only
// MigrationTimeout bounds it.
func TestMigrationsAreNotBoundByStartupDeadline(t *testing.T) {
	appURL, ownerURL := testdb.URLs(t)
	release := holdMigrationLock(t, ownerURL)

	original := startupTimeout
	startupTimeout = time.Second
	t.Cleanup(func() { startupTimeout = original })

	timer := time.AfterFunc(2*time.Second, release)
	defer timer.Stop()

	srv, err := New(context.Background(), config.Config{
		DatabaseURL:          appURL,
		MigrationDatabaseURL: ownerURL,
		Port:                 "0",
		MigrationTimeout:     time.Minute,
		JWTSecret:            testJWTSecret,
	})
	if err != nil {
		t.Fatalf("New = %v, want success once the lock is released", err)
	}
	srv.pool.Close()
}

// Refusal comes before migrations, so a misconfigured server changes nothing.
func TestNewRefusesPrivilegedAppRole(t *testing.T) {
	_, ownerURL := testdb.URLs(t)
	holdMigrationLock(t, ownerURL)

	_, err := New(context.Background(), config.Config{
		DatabaseURL:          ownerURL,
		MigrationDatabaseURL: ownerURL,
		Port:                 "0",
		MigrationTimeout:     30 * time.Second,
		JWTSecret:            testJWTSecret,
	})
	if !errors.Is(err, store.ErrPrivilegedRole) {
		t.Fatalf("New = %v, want ErrPrivilegedRole", err)
	}
}

func TestNewRefusesWeakJWTSecret(t *testing.T) {
	_, err := New(context.Background(), config.Config{
		DatabaseURL:          "postgres://crm:x@127.0.0.1:1/crm",
		MigrationDatabaseURL: "postgres://crm:x@127.0.0.1:1/crm",
		Port:                 "0",
		MigrationTimeout:     time.Minute,
		JWTSecret:            "short",
	})
	if !errors.Is(err, auth.ErrWeakSecret) {
		t.Fatalf("New = %v, want ErrWeakSecret", err)
	}
}
