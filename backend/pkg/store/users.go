package store

import (
	"context"
	"errors"
	"fmt"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	pgUniqueViolation = "23505"
	pgInvalidText     = "22P02"
)

var (
	// ErrEmailTaken means another user has the email, compared case-insensitively.
	ErrEmailTaken = errors.New("email already registered")
	// ErrUserNotFound means no user matches.
	ErrUserNotFound = errors.New("user not found")
)

const userColumns = `id::text, email, name, created_at, updated_at`

func scanUser(row pgx.Row, extra ...any) (model.User, error) {
	var u model.User
	dest := append([]any{&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return model.User{}, err
	}
	return u, nil
}

// CreateUser inserts a user, or returns ErrEmailTaken.
func (s *Store) CreateUser(ctx context.Context, email, name, passwordHash string) (model.User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (email, name, password_hash) VALUES ($1, $2, $3) RETURNING `+userColumns,
		email, name, passwordHash))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return model.User{}, ErrEmailTaken
	}
	if err != nil {
		return model.User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

// UserCredentialsByEmail returns the user and password hash for an email,
// compared case-insensitively, or ErrUserNotFound.
func (s *Store) UserCredentialsByEmail(ctx context.Context, email string) (model.User, string, error) {
	var hash string
	u, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM users WHERE lower(email) = lower($1)`, email), &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, "", ErrUserNotFound
	}
	if err != nil {
		return model.User{}, "", fmt.Errorf("read user: %w", err)
	}
	return u, hash, nil
}

// UserByID returns a user, or ErrUserNotFound. An ID that is not a UUID matches
// nobody.
func (s *Store) UserByID(ctx context.Context, id string) (model.User, error) {
	return userByID(ctx, s.pool, id)
}

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func userByID(ctx context.Context, q queryRower, id string) (model.User, error) {
	u, err := scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1::text::uuid`, id))
	var pgErr *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pgErr) && pgErr.Code == pgInvalidText) {
		return model.User{}, ErrUserNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("read user: %w", err)
	}
	return u, nil
}
