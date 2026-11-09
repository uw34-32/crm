package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-that-is-long-enough-for-hs256"

func newManager(t *testing.T, ttl time.Duration) *Manager {
	t.Helper()
	m, err := NewManager(testSecret, ttl)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNewManagerRefusesShortSecret(t *testing.T) {
	for _, secret := range []string{"", "short", strings.Repeat("x", MinSecretBytes-1)} {
		if _, err := NewManager(secret, time.Minute); err == nil {
			t.Errorf("NewManager(%d bytes) succeeded, want an error", len(secret))
		}
	}
	if _, err := NewManager(strings.Repeat("x", MinSecretBytes), time.Minute); err != nil {
		t.Errorf("NewManager(%d bytes) = %v, want success", MinSecretBytes, err)
	}
}

func TestVerifyAcceptsGeneratedToken(t *testing.T) {
	m := newManager(t, time.Minute)
	token, err := m.Generate("user-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != "user-1" {
		t.Errorf("subject = %q, want user-1", got)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	m := newManager(t, -time.Minute)
	token, err := m.Generate("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Verify(token); err == nil {
		t.Fatal("Verify accepted an expired token")
	}
}

func TestVerifyRejectsBadTokens(t *testing.T) {
	valid := func() jwt.RegisteredClaims {
		return jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(t *testing.T, method jwt.SigningMethod, claims jwt.Claims, key any) string {
		t.Helper()
		s, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	noExpiry := valid()
	noExpiry.ExpiresAt = nil
	wrongIssuer := valid()
	wrongIssuer.Issuer = "someone-else"
	noSubject := valid()
	noSubject.Subject = ""

	tests := []struct {
		name  string
		token string
	}{
		{"alg none", sign(t, jwt.SigningMethodNone, valid(), jwt.UnsafeAllowNoneSignatureType)},
		{"HS512 with the right secret", sign(t, jwt.SigningMethodHS512, valid(), []byte(testSecret))},
		{"RS256", sign(t, jwt.SigningMethodRS256, valid(), rsaKey)},
		{"wrong secret", sign(t, jwt.SigningMethodHS256, valid(), []byte(testSecret+"x"))},
		{"no expiry", sign(t, jwt.SigningMethodHS256, noExpiry, []byte(testSecret))},
		{"wrong issuer", sign(t, jwt.SigningMethodHS256, wrongIssuer, []byte(testSecret))},
		{"no subject", sign(t, jwt.SigningMethodHS256, noSubject, []byte(testSecret))},
		{"garbage", "not-a-token"},
		{"empty", ""},
	}
	m := newManager(t, time.Minute)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if sub, err := m.Verify(tt.token); err == nil {
				t.Fatalf("Verify accepted the token for subject %q", sub)
			}
		})
	}
}
