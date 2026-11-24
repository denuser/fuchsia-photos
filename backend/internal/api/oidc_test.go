package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
	"github.com/magellanica-app/magellanica-photos/backend/internal/config"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

const (
	fakeClientID  = "client"
	fakePublicURL = "https://photos.test"
)

// fakeProvider is an OpenID Connect provider with a discovery document, keys and a token
// endpoint, which answers with an ID token made from the claims the test sets.
type fakeProvider struct {
	*httptest.Server
	key    *rsa.PrivateKey
	claims map[string]any
	// nonce is what the token carries, set from the sign-in's redirect unless a test changes it.
	nonce string
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fp := &fakeProvider{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                fp.URL,
			"authorization_endpoint":                fp.URL + "/authorize",
			"token_endpoint":                        fp.URL + "/token",
			"jwks_uri":                              fp.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
		}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		claims := map[string]any{
			"iss": fp.URL, "aud": fakeClientID, "nonce": fp.nonce,
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
		}
		for k, v := range fp.claims {
			claims[k] = v
		}
		payload, _ := json.Marshal(claims)
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
			(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
		if err != nil {
			t.Error(err)
		}
		signed, err := signer.Sign(payload)
		if err != nil {
			t.Error(err)
		}
		idToken, _ := signed.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "token_type": "Bearer", "id_token": idToken})
	})
	fp.Server = httptest.NewServer(mux)
	t.Cleanup(fp.Close)
	return fp
}

func setupOIDC(t *testing.T, fp *fakeProvider) *env {
	t.Helper()
	return setupWith(t, func(d *Deps) {
		d.PublicURL = fakePublicURL
		d.SessionSecret = "0123456789abcdef0123456789abcdef"
		d.OIDCProviders = []config.OIDCProvider{{ID: "1", Name: "Fake", Issuer: fp.URL, ClientID: fakeClientID, ClientSecret: "secret"}}
	})
}

// browserNoRedirects shows each redirect to the test instead of following it.
func (e *env) browserNoRedirects() *browser {
	b := e.browser()
	b.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return b
}

// start begins a sign-in or a link and returns the state the provider would send back.
func (fp *fakeProvider) start(t *testing.T, b *browser, query string) string {
	t.Helper()
	res := b.get("/api/v1/auth/oidc/1/login" + query)
	wantStatus(t, res, http.StatusSeeOther)
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := loc.Query()
	if loc.Path != "/authorize" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" ||
		q.Get("redirect_uri") != fakePublicURL+"/api/v1/auth/oidc/1/callback" || q.Get("client_id") != fakeClientID {
		t.Fatalf("redirect to the provider = %s", loc)
	}
	fp.nonce = q.Get("nonce")
	return q.Get("state")
}

// signIn runs a whole sign-in and returns where the browser is sent at the end.
func (fp *fakeProvider) signIn(t *testing.T, b *browser, claims map[string]any) string {
	t.Helper()
	fp.claims = claims
	state := fp.start(t, b, "")
	return fp.finish(t, b, state)
}

func (fp *fakeProvider) finish(t *testing.T, b *browser, state string) string {
	t.Helper()
	res := b.get("/api/v1/auth/oidc/1/callback?code=good-code&state=" + url.QueryEscape(state))
	wantStatus(t, res, http.StatusSeeOther)
	return res.Header.Get("Location")
}

func (e *env) allowSignIn(create bool, allowed ...string) {
	e.t.Helper()
	if allowed == nil {
		allowed = []string{}
	}
	_, err := e.queries.UpdateSignInSettings(context.Background(), store.UpdateSignInSettingsParams{CreateUsers: create, Allowed: allowed})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) userCount() int {
	e.t.Helper()
	users, err := e.queries.ListUsers(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	return len(users)
}

func TestOIDCSignInCreatesAUserOnce(t *testing.T) {
	fp := newFakeProvider(t)
	e := setupOIDC(t, fp)
	e.allowSignIn(true)
	claims := map[string]any{"sub": "s1", "email": "Ann@Example.org", "email_verified": true, "preferred_username": "ann"}

	b := e.browserNoRedirects()
	if loc := fp.signIn(t, b, claims); loc != "/" {
		t.Fatalf("redirected to %q, want /", loc)
	}
	me := decode[userJSON](t, b.get("/api/v1/me"))
	if me.Login != "ann" || me.Role != auth.RoleUser {
		t.Errorf("user = %+v, want ann with the role user", me)
	}
	user, err := e.queries.GetUserByLogin(context.Background(), "ann")
	if err != nil {
		t.Fatal(err)
	}
	if user.PasswordHash != "" || user.Email != "ann@example.org" {
		t.Errorf("hash = %q, email = %q, want no password and the email in lower case", user.PasswordHash, user.Email)
	}
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login", map[string]string{"login": "ann", "password": ""}), http.StatusUnauthorized)

	again := e.browserNoRedirects()
	if loc := fp.signIn(t, again, claims); loc != "/" {
		t.Fatalf("second sign-in redirected to %q", loc)
	}
	if got := decode[userJSON](t, again.get("/api/v1/me")).ID; got != me.ID || e.userCount() != 1 {
		t.Errorf("the second sign-in made another user: id %d, %d users", got, e.userCount())
	}
}

func TestOIDCSignInRefusals(t *testing.T) {
	fp := newFakeProvider(t)
	e := setupOIDC(t, fp)
	claims := func(sub, email string, verified bool) map[string]any {
		return map[string]any{"sub": sub, "email": email, "email_verified": verified}
	}
	wantRefused := func(b *browser, loc, reason string) {
		t.Helper()
		if loc != "/login?error=oidc_"+reason {
			t.Errorf("redirected to %q, want the reason %s", loc, reason)
		}
		wantStatus(t, b.get("/api/v1/me"), http.StatusUnauthorized)
	}

	b := e.browserNoRedirects()
	wantRefused(b, fp.signIn(t, b, claims("s1", "a@family.example", true)), "no_account")

	e.allowSignIn(true, "family.example", "ben@gmail.com")
	b = e.browserNoRedirects()
	wantRefused(b, fp.signIn(t, b, claims("s2", "x@gmail.com", true)), "not_allowed")
	b = e.browserNoRedirects()
	wantRefused(b, fp.signIn(t, b, claims("s3", "a@family.example", false)), "not_allowed")
	b = e.browserNoRedirects()
	wantRefused(b, fp.signIn(t, b, map[string]any{"sub": "s4"}), "not_allowed")
	if e.userCount() != 0 {
		t.Fatalf("%d users made by refused sign-ins", e.userCount())
	}

	b = e.browserNoRedirects()
	if loc := fp.signIn(t, b, claims("s5", "A@Family.Example", true)); loc != "/" {
		t.Errorf("a domain on the list: redirected to %q", loc)
	}
	b = e.browserNoRedirects()
	if loc := fp.signIn(t, b, claims("s6", "ben@gmail.com", true)); loc != "/" {
		t.Errorf("an address on the list: redirected to %q", loc)
	}

	// An account that exists, switched off, stays shut.
	found, err := e.queries.GetUserByIdentity(context.Background(), store.GetUserByIdentityParams{Issuer: fp.URL, Subject: "s5"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.queries.SetUserDisabled(context.Background(), store.SetUserDisabledParams{ID: found.User.ID, Disabled: true}); err != nil {
		t.Fatal(err)
	}
	b = e.browserNoRedirects()
	wantRefused(b, fp.signIn(t, b, claims("s5", "a@family.example", true)), "disabled")
}

func TestOIDCSignInChecksTheReturn(t *testing.T) {
	fp := newFakeProvider(t)
	e := setupOIDC(t, fp)
	e.allowSignIn(true)
	fp.claims = map[string]any{"sub": "s1", "preferred_username": "ann"}

	b := e.browserNoRedirects()
	state := fp.start(t, b, "")
	if loc := fp.finish(t, b, "another-state"); loc != "/login?error=oidc_expired" {
		t.Errorf("a wrong state: redirected to %q", loc)
	}
	// The cookie is spent by the first return, even a failed one.
	if loc := fp.finish(t, b, state); loc != "/login?error=oidc_expired" {
		t.Errorf("a second return: redirected to %q", loc)
	}
	if loc := fp.finish(t, e.browserNoRedirects(), state); loc != "/login?error=oidc_expired" {
		t.Errorf("a return with no cookie: redirected to %q", loc)
	}

	b = e.browserNoRedirects()
	state = fp.start(t, b, "")
	fp.nonce = "another-nonce"
	if loc := fp.finish(t, b, state); loc != "/login?error=oidc_denied" {
		t.Errorf("a wrong nonce: redirected to %q", loc)
	}

	b = e.browserNoRedirects()
	state = fp.start(t, b, "")
	res := b.get("/api/v1/auth/oidc/1/callback?error=access_denied&state=" + url.QueryEscape(state))
	if loc := res.Header.Get("Location"); loc != "/login?error=oidc_denied" {
		t.Errorf("a refusal by the provider: redirected to %q", loc)
	}

	b = e.browserNoRedirects()
	state = fp.start(t, b, "")
	res = b.get("/api/v1/auth/oidc/1/callback?code=bad-code&state=" + url.QueryEscape(state))
	if loc := res.Header.Get("Location"); loc != "/login?error=oidc_denied" {
		t.Errorf("a code the provider refuses: redirected to %q", loc)
	}
	if e.userCount() != 0 {
		t.Errorf("%d users made by failed returns", e.userCount())
	}
	wantStatus(t, b.get("/api/v1/auth/oidc/9/login"), http.StatusNotFound)
}

func TestOIDCLoginsForCreatedUsers(t *testing.T) {
	fp := newFakeProvider(t)
	e := setupOIDC(t, fp)
	e.allowSignIn(true)
	e.addUser("ann", auth.RoleUser)

	login := func(sub string, claims map[string]any) string {
		t.Helper()
		claims["sub"] = sub
		b := e.browserNoRedirects()
		fp.signIn(t, b, claims)
		return decode[userJSON](t, b.get("/api/v1/me")).Login
	}
	if got := login("s1", map[string]any{"preferred_username": "Ann"}); got != "ann-2" {
		t.Errorf("a taken login: got %q, want ann-2", got)
	}
	if got := login("s2", map[string]any{"email": "carl.d@example.org"}); got != "carl.d" {
		t.Errorf("from the email: got %q", got)
	}
	if got := login("s3", map[string]any{"name": "Dora Díaz"}); got != "dora_daz" {
		t.Errorf("from the name: got %q", got)
	}
	got := login("s4", map[string]any{})
	if auth.ValidateLogin(got) != nil || len(got) < 6 {
		t.Errorf("with no claims: got %q, want a valid random login", got)
	}
}

func TestOIDCLinkAndUnlink(t *testing.T) {
	fp := newFakeProvider(t)
	e := setupOIDC(t, fp)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	claims := map[string]any{"sub": "s1", "email": "anna@example.org", "email_verified": true}

	// Linking needs a session.
	wantStatus(t, e.browserNoRedirects().get("/api/v1/auth/oidc/1/login?link=1"), http.StatusUnauthorized)

	anna := e.loggedIn("anna")
	anna.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	fp.claims = claims
	if loc := fp.finish(t, anna, fp.start(t, anna, "?link=1")); loc != "/profile?linked=1" {
		t.Fatalf("a link: redirected to %q", loc)
	}
	identities := decode[[]identityJSON](t, anna.get("/api/v1/me/identities"))
	if len(identities) != 1 || identities[0].Email != "anna@example.org" || identities[0].Issuer != fp.URL {
		t.Fatalf("identities = %+v", identities)
	}

	// Now the provider signs her in, with no settings that create users.
	b := e.browserNoRedirects()
	if loc := fp.signIn(t, b, claims); loc != "/" {
		t.Fatalf("sign-in: redirected to %q", loc)
	}
	if decode[userJSON](t, b.get("/api/v1/me")).Login != "anna" {
		t.Error("the identity did not sign in as anna")
	}

	// The same identity cannot be linked to another account.
	boris := e.loggedIn("boris")
	boris.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if loc := fp.finish(t, boris, fp.start(t, boris, "?link=1")); loc != "/profile?error=oidc_taken" {
		t.Errorf("a taken identity: redirected to %q", loc)
	}

	// A link started by one user cannot be finished in another's session.
	state := fp.start(t, anna, "?link=1")
	other := e.loggedIn("boris")
	other.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, c := range anna.client.Jar.Cookies(mustURL(t, e.server.URL+oidcBasePath)) {
		if c.Name == oidcCookie {
			other.client.Jar.SetCookies(mustURL(t, e.server.URL), []*http.Cookie{c})
		}
	}
	if loc := fp.finish(t, other, state); loc == "/profile?linked=1" {
		t.Error("another user finished the link")
	}

	// Unlinking: a user with a password may.
	id := identities[0].ID
	wantStatus(t, boris.json(http.MethodDelete, "/api/v1/me/identities/"+strconv.FormatInt(id, 10), nil), http.StatusNotFound)
	wantStatus(t, anna.json(http.MethodDelete, "/api/v1/me/identities/"+strconv.FormatInt(id, 10), nil), http.StatusNoContent)
	wantStatus(t, anna.json(http.MethodDelete, "/api/v1/me/identities/"+strconv.FormatInt(id, 10), nil), http.StatusNotFound)
}

func TestOIDCOnlyWayInCannotBeUnlinked(t *testing.T) {
	fp := newFakeProvider(t)
	e := setupOIDC(t, fp)
	e.allowSignIn(true)
	b := e.browserNoRedirects()
	fp.signIn(t, b, map[string]any{"sub": "s1", "preferred_username": "ann"})

	identities := decode[[]identityJSON](t, b.get("/api/v1/me/identities"))
	wantStatus(t, b.json(http.MethodDelete, "/api/v1/me/identities/"+strconv.FormatInt(identities[0].ID, 10), nil), http.StatusConflict)
}

func TestSignInOptionsAndSettings(t *testing.T) {
	fp := newFakeProvider(t)
	e := setupOIDC(t, fp)
	e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)

	opts := decode[signInOptionsJSON](t, e.browser().get("/api/v1/auth/options"))
	if len(opts.Providers) != 1 || opts.Providers[0] != (signInProviderJSON{ID: "1", Name: "Fake"}) || opts.PasswordAdminsOnly {
		t.Errorf("options = %+v", opts)
	}

	anna, root := e.loggedIn("anna"), e.loggedIn("root")
	wantStatus(t, anna.get("/api/v1/settings/sign-in"), http.StatusForbidden)
	wantStatus(t, anna.json(http.MethodPut, "/api/v1/settings/sign-in", signInSettingsJSON{}), http.StatusForbidden)
	wantStatus(t, root.json(http.MethodPut, "/api/v1/settings/sign-in", signInSettingsJSON{Allowed: []string{"a b"}}), http.StatusBadRequest)
	wantStatus(t, root.json(http.MethodPut, "/api/v1/settings/sign-in", signInSettingsJSON{Allowed: []string{"@example.org"}}), http.StatusBadRequest)
	res := root.json(http.MethodPut, "/api/v1/settings/sign-in", signInSettingsJSON{
		CreateUsers: true, Allowed: []string{" Example.org ", "", "example.org", "Ann@x.org"}, PasswordAdminsOnly: true,
	})
	wantStatus(t, res, http.StatusOK)
	got := decode[signInSettingsJSON](t, root.get("/api/v1/settings/sign-in"))
	if !got.CreateUsers || !got.PasswordAdminsOnly || len(got.Allowed) != 2 || got.Allowed[0] != "example.org" || got.Allowed[1] != "ann@x.org" {
		t.Errorf("settings = %+v", got)
	}

	// With passwords closed to ordinary users, only an administrator signs in with one.
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login", map[string]string{"login": "anna", "password": "password anna"}), http.StatusUnauthorized)
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login", map[string]string{"login": "root", "password": "password root"}), http.StatusOK)
	if !decode[signInOptionsJSON](t, e.browser().get("/api/v1/auth/options")).PasswordAdminsOnly {
		t.Error("the options do not say that passwords are closed")
	}
}

func TestAdmittedAndSuggestLogin(t *testing.T) {
	if !admitted(nil, "") || admitted([]string{"x.org"}, "") || admitted([]string{"x.org"}, "a@y.org") || !admitted([]string{"a@y.org"}, "a@y.org") {
		t.Error("admitted gives a wrong answer")
	}
	if got := suggestLogin(oidcClaims{PreferredUsername: "Ann B", Email: "x@y.org"}, 0); got != "ann_b" {
		t.Errorf("suggestLogin = %q, want ann_b", got)
	}
	if got := suggestLogin(oidcClaims{PreferredUsername: "ann"}, 3); got != "ann-4" {
		t.Errorf("suggestLogin, fourth attempt = %q, want ann-4", got)
	}
	// Nothing usable in the claims, or too long a name: still a valid login.
	for _, c := range []oidcClaims{{PreferredUsername: "!", Email: "x@y.org"}, {PreferredUsername: strings.Repeat("a", 200)}} {
		for _, attempt := range []int{0, 12} {
			if got := suggestLogin(c, attempt); auth.ValidateLogin(got) != nil {
				t.Errorf("suggestLogin(%+v, %d) = %q, not a valid login", c, attempt, got)
			}
		}
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
