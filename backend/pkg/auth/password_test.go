package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func newPasswords(t *testing.T) *Passwords {
	t.Helper()
	p, err := NewPasswords(bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPasswordsVerifyMatchesOnlyTheHashedPassword(t *testing.T) {
	p := newPasswords(t)
	hash, err := p.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "correct") {
		t.Error("hash contains the password")
	}
	if !p.Verify(hash, "correct horse battery") {
		t.Error("Verify rejected the right password")
	}
	if p.Verify(hash, "wrong horse battery") {
		t.Error("Verify accepted the wrong password")
	}
}

func TestPasswordsHashRefusesTooLong(t *testing.T) {
	p := newPasswords(t)
	if _, err := p.Hash(strings.Repeat("x", MaxPasswordBytes)); err != nil {
		t.Errorf("Hash at the limit: %v", err)
	}
	if _, err := p.Hash(strings.Repeat("x", MaxPasswordBytes+1)); err == nil {
		t.Error("Hash accepted a password bcrypt would truncate")
	}
}

// bcrypt reads at most 72 bytes, so without the guard this would match.
func TestPasswordsVerifyRejectsTooLong(t *testing.T) {
	p := newPasswords(t)
	password := strings.Repeat("x", MaxPasswordBytes)
	hash, err := p.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if p.Verify(hash, password+"extra") {
		t.Error("Verify accepted a password beyond the limit")
	}
}

func TestPasswordsVerifyRejectsCorruptHash(t *testing.T) {
	if newPasswords(t).Verify("not-a-hash", "whatever-password") {
		t.Error("Verify accepted a corrupt hash")
	}
}

func TestPasswordsVerifyUnknownHandlesAnyLength(t *testing.T) {
	p := newPasswords(t)
	p.VerifyUnknown("")
	p.VerifyUnknown(strings.Repeat("x", 4096))
}

func TestNewPasswordsRefusesInvalidCost(t *testing.T) {
	if _, err := NewPasswords(bcrypt.MaxCost + 1); err == nil {
		t.Error("NewPasswords accepted a cost bcrypt refuses")
	}
}
