package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func codeFor(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestTOTPKeySealsSecrets(t *testing.T) {
	key, err := NewTOTPKey(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := key.Encrypt(7, "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := key.Decrypt(7, sealed); err != nil || got != "JBSWY3DPEHPK3PXP" {
		t.Errorf("Decrypt = %q, %v", got, err)
	}
	if _, err := key.Decrypt(8, sealed); err == nil {
		t.Error("a secret opened for another user")
	}
	other, _ := NewTOTPKey(strings.Repeat("o", 32))
	if _, err := other.Decrypt(7, sealed); err == nil {
		t.Error("a secret opened with another key")
	}
	if _, err := key.Decrypt(7, sealed[:5]); err == nil {
		t.Error("a truncated secret opened")
	}
	off, _ := NewTOTPKey("")
	if off.On() {
		t.Error("an empty setting turned the key on")
	}
	if _, err := off.Encrypt(7, "x"); err == nil {
		t.Error("encrypted without a key")
	}
}

func TestCheckTOTP(t *testing.T) {
	secret, uri, err := NewTOTPSecret("anna")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, "anna") {
		t.Errorf("uri = %s", uri)
	}
	now := time.Unix(1_800_000_010, 0)
	step := now.Unix() / 30
	for offset, wantOK := range map[int64]bool{-2: false, -1: true, 0: true, 1: true, 2: false} {
		code := codeFor(t, secret, time.Unix((step+offset)*30, 0))
		got, ok := CheckTOTP(secret, code, now)
		if ok != wantOK || (ok && got != step+offset) {
			t.Errorf("offset %d: step %d, ok %v, want ok %v", offset, got, ok, wantOK)
		}
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := CheckTOTP(secret, bad, now); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	// A space or dash the app shows between the halves is ignored.
	code := codeFor(t, secret, now)
	if _, ok := CheckTOTP(secret, code[:3]+" "+code[3:], now); !ok {
		t.Error("a code with a space was refused")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, hashes, err := NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != RecoveryCodeCount || len(hashes) != RecoveryCodeCount {
		t.Fatalf("%d codes, %d hashes", len(codes), len(hashes))
	}
	seen := map[string]bool{}
	for i, c := range codes {
		if len(c) != 11 || c[5] != '-' || !LooksLikeRecoveryCode(c) || seen[c] {
			t.Errorf("code %q is malformed or repeated", c)
		}
		seen[c] = true
		if string(HashRecoveryCode(strings.ToUpper(c))) != string(hashes[i]) || string(HashRecoveryCode(strings.ReplaceAll(c, "-", ""))) != string(hashes[i]) {
			t.Errorf("code %q does not hash the same in capitals or without the dash", c)
		}
	}
	if LooksLikeRecoveryCode("123456") {
		t.Error("a six-digit code looks like a recovery code")
	}
}

func TestPendingToken(t *testing.T) {
	key, _ := NewTOTPKey(strings.Repeat("k", 32))
	now := time.Unix(1_800_000_000, 0)
	token := key.NewPending(42, now)
	if id, ok := key.CheckPending(token, now.Add(PendingLifetime-time.Second)); !ok || id != 42 {
		t.Errorf("a fresh token: %d, %v", id, ok)
	}
	if _, ok := key.CheckPending(token, now.Add(PendingLifetime)); ok {
		t.Error("an expired token accepted")
	}
	other, _ := NewTOTPKey(strings.Repeat("o", 32))
	if _, ok := other.CheckPending(token, now); ok {
		t.Error("a token of another key accepted")
	}
	for _, bad := range []string{"", "x", token + "0", "." + token, token[:7] + "B" + token[8:]} {
		if _, ok := key.CheckPending(bad, now); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	off, _ := NewTOTPKey("")
	if _, ok := off.CheckPending(token, now); ok {
		t.Error("accepted with no key")
	}
}
