package auth

import (
	"bytes"
	"regexp"
	"testing"
)

func TestPassword(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse") {
		t.Error("the right password was rejected")
	}
	if CheckPassword(hash, "wrong horse") {
		t.Error("a wrong password was accepted")
	}
	if CheckPassword("not a hash", "correct horse") {
		t.Error("a malformed hash was accepted")
	}
}

func TestValidate(t *testing.T) {
	for _, login := range []string{"a", "has space", "../x", ""} {
		if ValidateLogin(login) == nil {
			t.Errorf("login %q was accepted", login)
		}
	}
	if err := ValidateLogin("anna.k-1"); err != nil {
		t.Error(err)
	}
	if ValidatePassword("short") == nil {
		t.Error("a short password was accepted")
	}
}

func TestNewAppPassword(t *testing.T) {
	format := regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{5}(-[0-9a-hjkmnp-tv-z]{5}){3}$`)
	seen := map[string]bool{}
	for range 100 {
		password, hash, err := NewAppPassword()
		if err != nil {
			t.Fatal(err)
		}
		if !format.MatchString(password) {
			t.Errorf("password %q does not have the expected format", password)
		}
		if !bytes.Equal(hash, HashAppPassword(password)) {
			t.Errorf("hash of %q does not match HashAppPassword", password)
		}
		if seen[password] {
			t.Errorf("password %q repeated", password)
		}
		seen[password] = true
	}
}
