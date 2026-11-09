package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"crm/pkg/model"
	"crm/pkg/testdb"
)

func uniqueEmail(t *testing.T) string {
	t.Helper()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	return "user-" + hex.EncodeToString(suffix) + "@example.test"
}

func newUser(t *testing.T, s *Store) model.User {
	t.Helper()
	u, err := s.CreateUser(context.Background(), uniqueEmail(t), "Test User", "stored-hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func TestCreateUserRefusesDuplicateEmailCaseInsensitively(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	email := uniqueEmail(t)
	if _, err := s.CreateUser(ctx, email, "A", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, strings.ToUpper(email), "B", "h"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

func TestUserLookups(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	created := newUser(t, s)

	got, hash, err := s.UserCredentialsByEmail(ctx, strings.ToUpper(created.Email))
	if err != nil {
		t.Fatalf("UserCredentialsByEmail: %v", err)
	}
	if got.ID != created.ID || hash != "stored-hash" {
		t.Errorf("got %+v and hash %q", got, hash)
	}
	byID, err := s.UserByID(ctx, created.ID)
	if err != nil || byID.Email != created.Email {
		t.Errorf("UserByID = %+v, %v", byID, err)
	}

	if _, _, err := s.UserCredentialsByEmail(ctx, uniqueEmail(t)); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown email: err = %v, want ErrUserNotFound", err)
	}
	if _, err := s.UserByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown id: err = %v, want ErrUserNotFound", err)
	}
	// A subject that is not a UUID is simply nobody, not a server error.
	if _, err := s.UserByID(ctx, "not-a-uuid"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("malformed id: err = %v, want ErrUserNotFound", err)
	}
}

const (
	testTTL = time.Hour
	noGrace = time.Duration(0)
)

func TestRefreshTokenStoredOnlyAsHash(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	raw, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	var exact, hashed bool
	err = s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM refresh_tokens WHERE encode(token_hash, 'escape') = $1),
		       EXISTS (SELECT 1 FROM refresh_tokens WHERE token_hash = sha256(convert_to($1, 'UTF8')))`,
		raw).Scan(&exact, &hashed)
	if err != nil {
		t.Fatal(err)
	}
	if exact {
		t.Error("raw token stored")
	}
	if !hashed {
		t.Error("SHA-256 of the token not stored")
	}
}

func TestRotateIssuesSuccessorAndRetiresToken(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	got, next, err := s.RotateRefreshToken(ctx, first, testTTL, noGrace)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("rotate returned user %s, want %s", got.ID, u.ID)
	}
	if next == "" || next == first {
		t.Error("rotate did not issue a new token")
	}
	if _, _, err := s.RotateRefreshToken(ctx, next, testTTL, noGrace); err != nil {
		t.Errorf("successor refused: %v", err)
	}
}

func TestRotateRefusesUnknownAndEmptyTokens(t *testing.T) {
	s := New(testdb.AppPool(t))
	for _, raw := range []string{"", "never-issued"} {
		if _, _, err := s.RotateRefreshToken(context.Background(), raw, testTTL, noGrace); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Errorf("Rotate(%q) = %v, want ErrInvalidRefreshToken", raw, err)
		}
	}
}

func TestReuseOutsideGraceRevokesFamily(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	_, successor, err := s.RotateRefreshToken(ctx, first, testTTL, noGrace)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.RotateRefreshToken(ctx, first, testTTL, noGrace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("reuse = %v, want ErrInvalidRefreshToken", err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, successor, testTTL, noGrace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("successor after reuse = %v, want the family revoked", err)
	}
	// A family is one login: another device keeps working.
	if _, _, err := s.RotateRefreshToken(ctx, other, testTTL, noGrace); err != nil {
		t.Errorf("another family was affected: %v", err)
	}
}

func TestRotateRefusesExpiredTokenWithoutRevokingFamily(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	raw, err := s.IssueRefreshToken(ctx, u.ID, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, raw, testTTL, noGrace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expired = %v, want ErrInvalidRefreshToken", err)
	}
	var revoked bool
	err = s.pool.QueryRow(ctx, `
		SELECT f.revoked_at IS NOT NULL FROM refresh_token_families f WHERE f.user_id = $1`, u.ID).Scan(&revoked)
	if err != nil {
		t.Fatal(err)
	}
	if revoked {
		t.Error("an expired token is not reuse, yet the family was revoked")
	}
}

func TestGraceRetryKeepsDeliveredSuccessorLive(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	_, delivered, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, retried, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour)
	if err != nil {
		t.Fatalf("retry inside grace: %v", err)
	}
	if retried == delivered {
		t.Fatal("the retry returned the same token")
	}
	// No grace here: the delivered token must work because it is live, not
	// because it counts as another retry.
	_, next, err := s.RotateRefreshToken(ctx, delivered, testTTL, noGrace)
	if err != nil {
		t.Fatalf("delivered successor refused after a retry: %v", err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, next, testTTL, noGrace); err != nil {
		t.Errorf("chain broken after a retry: %v", err)
	}
}

// Without convergence, a thief replaying a token inside the grace window would
// hold a second chain that never conflicts with the victim's.
func TestGraceRetryConvergesOnFirstRotation(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	_, a, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if live := liveTokens(t, s, u.ID); live != 2 {
		t.Fatalf("%d live tokens after a retry, want 2", live)
	}
	_, next, err := s.RotateRefreshToken(ctx, a, testTTL, noGrace)
	if err != nil {
		t.Fatal(err)
	}
	if live := liveTokens(t, s, u.ID); live != 1 {
		t.Fatalf("%d live tokens after the next rotation, want 1", live)
	}
	// The sibling is spent, so presenting it past grace is reuse.
	if _, _, err := s.RotateRefreshToken(ctx, b, testTTL, noGrace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("sibling = %v, want ErrInvalidRefreshToken", err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, next, testTTL, noGrace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("newest token = %v, want the family revoked", err)
	}
}

func liveTokens(t *testing.T, s *Store, userID string) int {
	t.Helper()
	var live int
	err := s.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM refresh_tokens t JOIN refresh_token_families f ON f.id = t.family_id
		 WHERE f.user_id = $1 AND t.rotated_at IS NULL`, userID).Scan(&live)
	if err != nil {
		t.Fatal(err)
	}
	return live
}

func TestParallelRefreshInsideGraceKeepsEveryDeliveredTokenUsable(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}

	const parallel = 8
	delivered := make([]string, parallel)
	errs := make([]error, parallel)
	var wg sync.WaitGroup
	for i := range parallel {
		wg.Go(func() {
			_, delivered[i], errs[i] = s.RotateRefreshToken(ctx, first, testTTL, time.Hour)
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("parallel refresh %d: %v", i, err)
		}
	}
	// The client keeps whichever response it saw last, and no order is promised.
	if _, _, err := s.RotateRefreshToken(ctx, delivered[0], testTTL, noGrace); err != nil {
		t.Errorf("a delivered token was refused: %v", err)
	}
}

// Once a successor has been spent the client has moved on, so a late copy of the
// old token earns nothing. Inside grace that is not proof of theft either, so it
// must not end the session.
func TestGraceRetryAfterSuccessorSpentIsRefusedWithoutRevoking(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, third, err := s.RotateRefreshToken(ctx, second, testTTL, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("late retry = %v, want ErrInvalidRefreshToken", err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, third, testTTL, noGrace); err != nil {
		t.Errorf("live token refused after a late retry: %v", err)
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })
	return &logs
}

func TestReuseRevocationLogsOneEventWithoutSecrets(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	_, successor, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	logs := captureLogs(t)
	// A retry inside grace is not reuse and logs nothing.
	if _, _, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Fatalf("a retry inside grace logged: %q", logs)
	}

	if _, _, err := s.RotateRefreshToken(ctx, first, testTTL, noGrace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("reuse = %v, want ErrInvalidRefreshToken", err)
	}
	line := logs.String()
	if !strings.Contains(line, "refresh_token_reuse") {
		t.Fatalf("reuse log lacks the event name: %q", line)
	}
	if !strings.Contains(line, u.ID) {
		t.Errorf("reuse log lacks the user ID: %q", line)
	}
	sum := sha256.Sum256([]byte(first))
	for name, secret := range map[string]string{
		"raw token":     first,
		"successor":     successor,
		"hash (hex)":    hex.EncodeToString(sum[:]),
		"hash (base64)": base64.RawURLEncoding.EncodeToString(sum[:]),
		"email":         u.Email,
	} {
		if strings.Contains(line, secret) {
			t.Errorf("reuse log exposes the %s", name)
		}
	}

	// The family is revoked now, so another attempt is not a new event.
	logs.Reset()
	if _, _, err := s.RotateRefreshToken(ctx, first, testTTL, noGrace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("second reuse = %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("an already revoked family logged again: %q", logs)
	}
}

func TestReuseAfterGraceExpiresRevokesFamily(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	const grace = 50 * time.Millisecond
	_, successor, err := s.RotateRefreshToken(ctx, first, testTTL, grace)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * grace)
	if _, _, err := s.RotateRefreshToken(ctx, first, testTTL, grace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("reuse = %v, want ErrInvalidRefreshToken", err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, successor, testTTL, grace); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("successor = %v, want the family revoked", err)
	}
}

func TestGraceDoesNotReviveRevokedFamily(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	first, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeRefreshFamily(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RotateRefreshToken(ctx, first, testTTL, time.Hour); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("retry after logout = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestRevokeUnknownTokenIsNoOp(t *testing.T) {
	s := New(testdb.AppPool(t))
	if err := s.RevokeRefreshFamily(context.Background(), "never-issued"); err != nil {
		t.Errorf("RevokeRefreshFamily = %v, want nil", err)
	}
}

// A held row lock makes the race deterministic. Holding the token row stalls
// every rotation at its write, after it has read the token. Without the family
// lock, all of them would read an unspent token before stalling, and all would
// then succeed.
func TestConcurrentRotationOutsideGraceHasOneWinner(t *testing.T) {
	pool := testdb.AppPool(t)
	s := New(pool)
	ctx := context.Background()
	u := newUser(t, s)
	raw, err := s.IssueRefreshToken(ctx, u.ID, testTTL)
	if err != nil {
		t.Fatal(err)
	}

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	if _, err := blocker.Exec(ctx, `SELECT 1 FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`, hashRefreshToken(raw)); err != nil {
		t.Fatal(err)
	}

	const racers = 12
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := range racers {
		wg.Go(func() {
			_, _, errs[i] = s.RotateRefreshToken(ctx, raw, testTTL, noGrace)
		})
	}
	time.Sleep(500 * time.Millisecond)
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, ErrInvalidRefreshToken):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Errorf("%d rotations succeeded, want exactly 1", wins)
	}
}

func TestIssuePurgesFamiliesLongExpired(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	u := newUser(t, s)
	if _, err := s.IssueRefreshToken(ctx, u.ID, -2*refreshPurgeAfter); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueRefreshToken(ctx, u.ID, testTTL); err != nil {
		t.Fatal(err)
	}
	var families int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM refresh_token_families WHERE user_id = $1`, u.ID).Scan(&families)
	if err != nil {
		t.Fatal(err)
	}
	if families != 1 {
		t.Errorf("%d families left, want the expired one purged", families)
	}
}
