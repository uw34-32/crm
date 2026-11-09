// Package auth issues and verifies access tokens and hashes passwords.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MinSecretBytes is the shortest accepted signing secret: HS256 needs a key at
// least as long as its 256-bit output.
const MinSecretBytes = 32

const issuer = "crm"

// ErrWeakSecret means the signing secret is shorter than MinSecretBytes.
var ErrWeakSecret = fmt.Errorf("JWT secret must be at least %d bytes", MinSecretBytes)

// Manager signs and verifies HS256 access tokens.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

func NewManager(secret string, ttl time.Duration) (*Manager, error) {
	if len(secret) < MinSecretBytes {
		return nil, ErrWeakSecret
	}
	return &Manager{secret: []byte(secret), ttl: ttl}, nil
}

// Generate returns a signed access token for the user.
func (m *Manager) Generate(userID string) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
	}).SignedString(m.secret)
}

// Verify returns the user ID in a valid token. Only HS256 is accepted, whatever
// the token's header claims, and the token must carry an expiry.
func (m *Manager) Verify(token string) (string, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return m.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", err
	}
	if claims.Subject == "" {
		return "", errors.New("token has no subject")
	}
	return claims.Subject, nil
}
