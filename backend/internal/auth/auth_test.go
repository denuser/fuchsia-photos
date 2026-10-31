package auth

import "testing"

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
