package auth

import (
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultCost is the bcrypt work factor for new hashes.
	DefaultCost = 12
	// MinPasswordBytes is the shortest accepted password.
	MinPasswordBytes = 8
	// MaxPasswordBytes is bcrypt's input limit; beyond it, input would be ignored.
	MaxPasswordBytes = 72
)

// ErrPasswordTooLong means the password exceeds MaxPasswordBytes.
var ErrPasswordTooLong = errors.New("password is too long")

// Passwords hashes and verifies passwords. It holds a dummy hash of the same
// cost as new hashes, so failed lookups can spend the same time as failed
// comparisons.
type Passwords struct {
	cost  int
	dummy []byte
}

func NewPasswords(cost int) (*Passwords, error) {
	dummy, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), cost)
	if err != nil {
		return nil, fmt.Errorf("create dummy password hash: %w", err)
	}
	return &Passwords{cost: cost, dummy: dummy}, nil
}

func (p *Passwords) Hash(password string) (string, error) {
	if len(password) > MaxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), p.cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Verify reports whether password matches hash. A password over the limit can
// never match, but still spends the time of a comparison.
func (p *Passwords) Verify(hash, password string) bool {
	if len(password) > MaxPasswordBytes {
		p.VerifyUnknown(password)
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// VerifyUnknown spends the time of a comparison without a real hash, so an
// unknown account cannot be told apart from a wrong password by timing.
func (p *Passwords) VerifyUnknown(password string) {
	input := []byte(password)
	if len(input) > MaxPasswordBytes {
		input = input[:MaxPasswordBytes]
	}
	_ = bcrypt.CompareHashAndPassword(p.dummy, input)
}
