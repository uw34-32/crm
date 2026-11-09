package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
)

// ErrInvalidRefreshToken covers every refresh failure: unknown, expired,
// revoked, or reused. Callers cannot tell the cases apart, so the table is not
// an existence oracle.
var ErrInvalidRefreshToken = errors.New("invalid refresh token")

// refreshPurgeAfter is how long past expiry a token is kept, so that presenting
// it again is still detected as reuse.
const refreshPurgeAfter = 7 * 24 * time.Hour

// hashRefreshToken returns the SHA-256 of a raw token. Tokens carry 256 random
// bits, so a fast unsalted hash is enough, and lookups by hash leave no secret
// comparison in application code.
func hashRefreshToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// insertRefreshToken stores the hash of a new random token and returns the raw
// token, which is never stored.
func insertRefreshToken(ctx context.Context, tx pgx.Tx, familyID string, now time.Time, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	if _, err := tx.Exec(ctx,
		`INSERT INTO refresh_tokens (token_hash, family_id, expires_at) VALUES ($1, $2::text::uuid, $3)`,
		hashRefreshToken(raw), familyID, now.Add(ttl)); err != nil {
		return "", fmt.Errorf("insert refresh token: %w", err)
	}
	return raw, nil
}

// IssueRefreshToken starts a new token family for a login and returns the raw
// token. It also purges the user's families that have long expired.
func (s *Store) IssueRefreshToken(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	var raw string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		now := time.Now()
		if _, err := tx.Exec(ctx, `
			DELETE FROM refresh_token_families f
			 WHERE f.user_id = $1::text::uuid
			   AND NOT EXISTS (SELECT 1 FROM refresh_tokens t
			                    WHERE t.family_id = f.id AND t.expires_at >= $2)`,
			userID, now.Add(-refreshPurgeAfter)); err != nil {
			return fmt.Errorf("purge refresh token families: %w", err)
		}

		var familyID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO refresh_token_families (user_id) VALUES ($1::text::uuid) RETURNING id::text`, userID,
		).Scan(&familyID); err != nil {
			return fmt.Errorf("insert refresh token family: %w", err)
		}

		var err error
		raw, err = insertRefreshToken(ctx, tx, familyID, now, ttl)
		return err
	})
	if err != nil {
		return "", err
	}
	return raw, nil
}

// RotateRefreshToken spends a token and issues its successor in the same family.
//
// Presenting a spent token revokes the whole family, because either the client
// or a thief holds a stale copy. The exception is a retry within grace of the
// rotation, which covers a client that lost the response: it gets a new
// successor and the previous one is spent, so one token stays live.
//
// A row lock on the family serializes rotations, so concurrent callers see each
// other's results and at most one can succeed outside the grace window.
func (s *Store) RotateRefreshToken(ctx context.Context, raw string, ttl, grace time.Duration) (model.User, string, error) {
	if raw == "" {
		return model.User{}, "", ErrInvalidRefreshToken
	}
	hash := hashRefreshToken(raw)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.User{}, "", err
	}
	defer tx.Rollback(ctx)

	// A token's family never changes, so reading it before the lock is safe.
	var familyID string
	err = tx.QueryRow(ctx, `SELECT family_id::text FROM refresh_tokens WHERE token_hash = $1`, hash).Scan(&familyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, "", ErrInvalidRefreshToken
	}
	if err != nil {
		return model.User{}, "", fmt.Errorf("read refresh token: %w", err)
	}

	var userID string
	var revoked bool
	err = tx.QueryRow(ctx,
		`SELECT user_id::text, revoked_at IS NOT NULL FROM refresh_token_families WHERE id = $1::text::uuid FOR UPDATE`,
		familyID).Scan(&userID, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, "", ErrInvalidRefreshToken
	}
	if err != nil {
		return model.User{}, "", fmt.Errorf("lock refresh token family: %w", err)
	}
	if revoked {
		return model.User{}, "", ErrInvalidRefreshToken
	}

	// Read after the lock, to see what a rotation that held it committed.
	var expiresAt time.Time
	var rotatedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT expires_at, rotated_at FROM refresh_tokens WHERE token_hash = $1`, hash).Scan(&expiresAt, &rotatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, "", ErrInvalidRefreshToken
	}
	if err != nil {
		return model.User{}, "", fmt.Errorf("read refresh token: %w", err)
	}

	now := time.Now()
	expired := !now.Before(expiresAt)
	if rotatedAt != nil && (expired || grace <= 0 || now.Sub(*rotatedAt) > grace) {
		if _, err := tx.Exec(ctx, `UPDATE refresh_token_families SET revoked_at = $2 WHERE id = $1::text::uuid`, familyID, now); err != nil {
			return model.User{}, "", fmt.Errorf("revoke refresh token family: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return model.User{}, "", fmt.Errorf("revoke refresh token family: %w", err)
		}
		return model.User{}, "", ErrInvalidRefreshToken
	}
	if expired {
		return model.User{}, "", ErrInvalidRefreshToken
	}

	user, err := userByID(ctx, tx, userID)
	if errors.Is(err, ErrUserNotFound) {
		return model.User{}, "", ErrInvalidRefreshToken
	}
	if err != nil {
		return model.User{}, "", err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET rotated_at = $2 WHERE family_id = $1::text::uuid AND rotated_at IS NULL`,
		familyID, now); err != nil {
		return model.User{}, "", fmt.Errorf("spend refresh token: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM refresh_tokens WHERE family_id = $1::text::uuid AND rotated_at IS NOT NULL AND expires_at < $2`,
		familyID, now.Add(-refreshPurgeAfter)); err != nil {
		return model.User{}, "", fmt.Errorf("purge refresh tokens: %w", err)
	}
	next, err := insertRefreshToken(ctx, tx, familyID, now, ttl)
	if err != nil {
		return model.User{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.User{}, "", fmt.Errorf("commit refresh token rotation: %w", err)
	}
	return user, next, nil
}

// RevokeRefreshFamily revokes the family of the presented token. An unknown
// token is a no-op, so the result reveals nothing about it.
func (s *Store) RevokeRefreshFamily(ctx context.Context, raw string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_token_families SET revoked_at = now()
		 WHERE revoked_at IS NULL
		   AND id = (SELECT family_id FROM refresh_tokens WHERE token_hash = $1)`,
		hashRefreshToken(raw))
	if err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return nil
}
