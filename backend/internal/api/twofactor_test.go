package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
	"github.com/magellanica-app/magellanica-photos/backend/internal/config"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

func testTOTPKey(t *testing.T, setting string) auth.TOTPKey {
	t.Helper()
	key, err := auth.NewTOTPKey(setting)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

const testTOTPSetting = "0123456789abcdef0123456789abcdef"

func setupTwoFactor(t *testing.T) *env {
	t.Helper()
	return setupWith(t, func(d *Deps) { d.TOTP = testTOTPKey(t, testTOTPSetting) })
}

// codeAt is the code an authenticator shows at a time.
func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return code
}

type setupResult struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	QR     string `json:"qr"`
}

// enableTwoFactor turns it on for a signed-in user. It returns the secret and the recovery codes.
// Enabling uses up the code of the current time step, so later sign-ins use the next step's code.
func enableTwoFactor(t *testing.T, b *browser) (string, []string) {
	t.Helper()
	setup := decode[setupResult](t, b.json(http.MethodPost, "/api/v1/me/2fa/setup", nil))
	res := b.json(http.MethodPost, "/api/v1/me/2fa/enable", map[string]string{"code": codeAt(t, setup.Secret, time.Now())})
	wantStatus(t, res, http.StatusOK)
	codes := decode[struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}](t, res).RecoveryCodes
	return setup.Secret, codes
}

// password signs in with the password only and returns the response.
func password(b *browser, login string) *http.Response {
	return b.json(http.MethodPost, "/api/v1/login", map[string]string{"login": login, "password": "password " + login})
}

type pending struct {
	TwoFactor bool   `json:"twoFactor"`
	Token     string `json:"token"`
}

func TestTwoFactorEnrolAndSignIn(t *testing.T) {
	e := setupTwoFactor(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	setup := decode[setupResult](t, anna.json(http.MethodPost, "/api/v1/me/2fa/setup", nil))
	if setup.Secret == "" || setup.URI == "" || len(setup.QR) < 100 || setup.QR[:22] != "data:image/png;base64," {
		t.Fatalf("setup = %+v", setup)
	}
	// Nothing changes for sign-in until a correct code arrives.
	if status := decode[twoFactorJSON](t, anna.get("/api/v1/me/2fa")); status.Enabled || !status.Available {
		t.Errorf("status before the code = %+v", status)
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/2fa/enable", map[string]string{"code": "000000"}), http.StatusUnauthorized)
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login", map[string]string{"login": "anna", "password": "password anna"}), http.StatusOK)

	enabledAt := time.Now()
	res := anna.json(http.MethodPost, "/api/v1/me/2fa/enable", map[string]string{"code": codeAt(t, setup.Secret, enabledAt)})
	wantStatus(t, res, http.StatusOK)
	codes := decode[struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}](t, res).RecoveryCodes
	if len(codes) != auth.RecoveryCodeCount {
		t.Fatalf("%d recovery codes, want %d", len(codes), auth.RecoveryCodeCount)
	}
	if status := decode[twoFactorJSON](t, anna.get("/api/v1/me/2fa")); !status.Enabled || status.RecoveryCodesLeft != auth.RecoveryCodeCount {
		t.Errorf("status = %+v", status)
	}
	if !decode[userJSON](t, anna.get("/api/v1/me")).TwoFactor {
		t.Error("the user does not say two-factor sign-in is on")
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/2fa/setup", nil), http.StatusConflict)

	// The password alone now gives a token and no session.
	other := e.browser()
	res = password(other, "anna")
	wantStatus(t, res, http.StatusAccepted)
	if len(res.Cookies()) != 0 {
		t.Errorf("cookies after the password: %v", res.Cookies())
	}
	first := decode[pending](t, res)
	if !first.TwoFactor || first.Token == "" {
		t.Fatalf("pending = %+v", first)
	}
	wantStatus(t, other.get("/api/v1/me"), http.StatusUnauthorized)

	// A wrong code and a wrong token are refused.
	wantStatus(t, other.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": first.Token, "code": "000000"}), http.StatusUnauthorized)
	wantStatus(t, other.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": first.Token + "x", "code": codeAt(t, setup.Secret, enabledAt.Add(30*time.Second))}), http.StatusUnauthorized)

	// The code of the step used to turn it on is spent; the next step's code works once.
	wantStatus(t, other.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": first.Token, "code": codeAt(t, setup.Secret, enabledAt)}), http.StatusUnauthorized)
	next := codeAt(t, setup.Secret, enabledAt.Add(30*time.Second))
	wantStatus(t, other.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": first.Token, "code": next}), http.StatusOK)
	wantStatus(t, other.get("/api/v1/me"), http.StatusOK)

	again := e.browser()
	second := decode[pending](t, password(again, "anna"))
	wantStatus(t, again.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": second.Token, "code": next}), http.StatusUnauthorized)

	// A recovery code works once, with or without the dash and in capitals.
	wantStatus(t, again.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": second.Token, "code": codes[0]}), http.StatusOK)
	third := decode[pending](t, password(e.browser(), "anna"))
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": third.Token, "code": codes[0]}), http.StatusUnauthorized)
	fourth := e.browser()
	wantStatus(t, fourth.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": decode[pending](t, password(fourth, "anna")).Token, "code": " " + upper(codes[1]) + " "}), http.StatusOK)
	if status := decode[twoFactorJSON](t, anna.get("/api/v1/me/2fa")); status.RecoveryCodesLeft != auth.RecoveryCodeCount-2 {
		t.Errorf("recovery codes left = %d, want %d", status.RecoveryCodesLeft, auth.RecoveryCodeCount-2)
	}

	// Changing the password keeps two-factor sign-in.
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/password", map[string]string{"currentPassword": "password anna", "newPassword": "a new password"}), http.StatusNoContent)
	res = e.browser().json(http.MethodPost, "/api/v1/login", map[string]string{"login": "anna", "password": "a new password"})
	wantStatus(t, res, http.StatusAccepted)
}

func upper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 32
		}
	}
	return string(out)
}

func TestTwoFactorPendingTokenExpires(t *testing.T) {
	key := testTOTPKey(t, testTOTPSetting)
	e := setupWith(t, func(d *Deps) { d.TOTP = key })
	user := e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	secret, _ := enableTwoFactor(t, anna)

	code := codeAt(t, secret, time.Now().Add(30*time.Second))
	old := key.NewPending(user.ID, time.Now().Add(-auth.PendingLifetime-time.Second))
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": old, "code": code}), http.StatusUnauthorized)
	// A token made under another key is no token.
	forged := testTOTPKey(t, "another key another key another key!").NewPending(user.ID, time.Now())
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": forged, "code": code}), http.StatusUnauthorized)
	fresh := key.NewPending(user.ID, time.Now())
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": fresh, "code": code}), http.StatusOK)
}

func TestTwoFactorSlowsDownWrongCodes(t *testing.T) {
	e := setupTwoFactor(t)
	e.addUser("anna", auth.RoleUser)
	secret, _ := enableTwoFactor(t, e.loggedIn("anna"))

	b := e.browser()
	token := decode[pending](t, password(b, "anna")).Token
	for range 5 {
		wantStatus(t, b.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": token, "code": "000000"}), http.StatusUnauthorized)
	}
	// Now even the right code waits, and says for how long.
	res := b.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": token, "code": codeAt(t, secret, time.Now().Add(30*time.Second))})
	wantStatus(t, res, http.StatusTooManyRequests)
	if res.Header.Get("Retry-After") == "" {
		t.Error("no Retry-After header")
	}
}

func TestTwoFactorTurnOff(t *testing.T) {
	e := setupTwoFactor(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	secret, codes := enableTwoFactor(t, anna)

	// It asks for the password and a code.
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/2fa/disable", map[string]string{"password": "wrong", "code": codes[0]}), http.StatusForbidden)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/2fa/disable", map[string]string{"password": "password anna", "code": "000000"}), http.StatusUnauthorized)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/2fa/disable", map[string]string{"password": "password anna", "code": codeAt(t, secret, time.Now().Add(30*time.Second))}), http.StatusNoContent)

	user, err := e.queries.GetUserByLogin(context.Background(), "anna")
	if err != nil {
		t.Fatal(err)
	}
	if user.TotpEnabled || user.TotpSecret != nil {
		t.Errorf("after turning off: enabled = %v, secret kept = %v", user.TotpEnabled, user.TotpSecret != nil)
	}
	if left, _ := e.queries.CountRecoveryCodes(context.Background(), user.ID); left != 0 {
		t.Errorf("%d recovery codes kept", left)
	}
	wantStatus(t, password(e.browser(), "anna"), http.StatusOK)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/2fa/disable", map[string]string{"password": "password anna", "code": codes[0]}), http.StatusConflict)
}

func TestTwoFactorAdministratorReset(t *testing.T) {
	e := setupTwoFactor(t)
	rootUser := e.addUser("root", auth.RoleAdmin)
	anna := e.addUser("anna", auth.RoleUser)
	session := e.loggedIn("anna")
	enableTwoFactor(t, session)
	wantStatus(t, password(e.browser(), "anna"), http.StatusAccepted)

	wantStatus(t, session.json(http.MethodPatch, "/api/v1/users/1", map[string]bool{"resetTwoFactor": true}), http.StatusForbidden)
	root := e.loggedIn("root")
	// Nobody turns off their own, not even an administrator: that takes the password and a code.
	wantStatus(t, root.json(http.MethodPatch, "/api/v1/users/"+itoa(rootUser.ID), map[string]bool{"resetTwoFactor": true}), http.StatusConflict)
	res := root.json(http.MethodPatch, "/api/v1/users/"+itoa(anna.ID), map[string]bool{"resetTwoFactor": true})
	wantStatus(t, res, http.StatusOK)
	if decode[userJSON](t, res).TwoFactor {
		t.Error("the user still has two-factor sign-in")
	}
	wantStatus(t, password(e.browser(), "anna"), http.StatusOK)
}

func TestTwoFactorNeedsTheKeyAndAPassword(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	if status := decode[twoFactorJSON](t, anna.get("/api/v1/me/2fa")); status.Available {
		t.Error("two-factor sign-in is available without TOTP_KEY")
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/2fa/setup", nil), http.StatusConflict)

	// A user with no password has nothing for a second factor to protect.
	e = setupTwoFactor(t)
	user := e.addUser("boris", auth.RoleUser)
	boris := e.loggedIn("boris")
	if err := e.queries.SetUserPassword(context.Background(), store.SetUserPasswordParams{ID: user.ID, PasswordHash: ""}); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, boris.json(http.MethodPost, "/api/v1/me/2fa/setup", nil), http.StatusConflict)
}

func TestTwoFactorSkippedByOIDC(t *testing.T) {
	fp := newFakeProvider(t)
	e := setupWith(t, func(d *Deps) {
		d.TOTP = testTOTPKey(t, testTOTPSetting)
		d.PublicURL = fakePublicURL
		d.SessionSecret = "0123456789abcdef0123456789abcdef"
		d.OIDCProviders = []config.OIDCProvider{{ID: "1", Name: "Fake", Issuer: fp.URL, ClientID: fakeClientID, ClientSecret: "secret"}}
	})
	user := e.addUser("anna", auth.RoleUser)
	enableTwoFactor(t, e.loggedIn("anna"))
	err := e.queries.CreateIdentity(context.Background(), store.CreateIdentityParams{UserID: user.ID, Issuer: fp.URL, Subject: "s1", Email: ""})
	if err != nil {
		t.Fatal(err)
	}

	b := e.browserNoRedirects()
	if loc := fp.signIn(t, b, map[string]any{"sub": "s1"}); loc != "/" {
		t.Fatalf("redirected to %q, want /", loc)
	}
	if got := decode[userJSON](t, b.get("/api/v1/me")).Login; got != "anna" {
		t.Errorf("signed in as %q", got)
	}
}

func TestUnreadableTwoFactorIsTurnedOffAtStart(t *testing.T) {
	e := setupTwoFactor(t)
	anna := e.addUser("anna", auth.RoleUser)
	boris := e.addUser("boris", auth.RoleUser)
	enableTwoFactor(t, e.loggedIn("anna"))
	enableTwoFactor(t, e.loggedIn("boris"))
	ctx := context.Background()

	// With the right key nothing changes.
	if err := DisableUnreadableTwoFactor(ctx, e.queries, testTOTPKey(t, testTOTPSetting)); err != nil {
		t.Fatal(err)
	}
	if u, _ := e.queries.GetUser(ctx, anna.ID); !u.TotpEnabled {
		t.Fatal("turned off with the right key")
	}
	// After a restore without the key, or with another one, nobody could sign in, so it is turned off.
	if err := DisableUnreadableTwoFactor(ctx, e.queries, testTOTPKey(t, "")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{anna.ID, boris.ID} {
		if u, _ := e.queries.GetUser(ctx, id); u.TotpEnabled || u.TotpSecret != nil {
			t.Errorf("user %d keeps two-factor sign-in without the key", id)
		}
	}
}

func TestTwoFactorLimitIsNotReachedThroughLogins(t *testing.T) {
	e := setupTwoFactor(t)
	user := e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	secret, _ := enableTwoFactor(t, anna)

	// A login that spells the code step's key must not lock it.
	b := e.browser()
	for range 6 {
		b.json(http.MethodPost, "/api/v1/login", map[string]string{"login": "totp:" + itoa(user.ID), "password": "wrong"})
	}
	token := decode[pending](t, password(e.browser(), "anna")).Token
	wantStatus(t, b.json(http.MethodPost, "/api/v1/login/totp", map[string]string{"token": token, "code": codeAt(t, secret, time.Now().Add(30*time.Second))}), http.StatusOK)
}
