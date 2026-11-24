package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
	"github.com/magellanica-app/magellanica-photos/backend/internal/config"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

const (
	oidcCookie   = "magellanica_oidc"
	oidcLifetime = 10 * time.Minute
	oidcBasePath = "/api/v1/auth/oidc"
)

// oidcProvider is one configured provider. Discovery runs on first use, so the application
// starts when a provider is down, and its result is kept once it succeeds.
type oidcProvider struct {
	config.OIDCProvider
	mu       sync.Mutex
	provider *oidc.Provider
}

func (p *oidcProvider) discover(ctx context.Context) (*oidc.Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil {
		return p.provider, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, p.Issuer)
	if err != nil {
		return nil, err
	}
	p.provider = provider
	return provider, nil
}

func (s *server) oidcProvider(id string) *oidcProvider {
	for _, p := range s.oidc {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (s *server) redirectURL(p *oidcProvider) string {
	return s.PublicURL + oidcBasePath + "/" + p.ID + "/callback"
}

// oidcState is what a sign-in remembers between the redirect to the provider and the return.
type oidcState struct {
	Provider string `json:"p"`
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	// LinkUser is the user who links the identity to their account, or zero for a sign-in.
	LinkUser int64 `json:"l,omitempty"`
	Expires  int64 `json:"e"`
}

func (s *server) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(s.SessionSecret))
	mac.Write([]byte("oidc-state:" + payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *server) sealState(st oidcState) (string, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + s.sign(payload), nil
}

func (s *server) openState(value string) (oidcState, bool) {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(payload))) {
		return oidcState{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return oidcState{}, false
	}
	var st oidcState
	if json.Unmarshal(raw, &st) != nil || time.Now().Unix() > st.Expires {
		return oidcState{}, false
	}
	return st, true
}

func (s *server) setStateCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcCookie,
		Value:    value,
		Path:     oidcBasePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

type signInProviderJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type signInOptionsJSON struct {
	Providers          []signInProviderJSON `json:"providers"`
	PasswordAdminsOnly bool                 `json:"passwordAdminsOnly"`
}

// listSignInOptions tells the sign-in page which providers to offer.
func (s *server) listSignInOptions(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Queries.GetSignInSettings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := signInOptionsJSON{Providers: []signInProviderJSON{}, PasswordAdminsOnly: settings.PasswordAdminsOnly}
	for _, p := range s.oidc {
		out.Providers = append(out.Providers, signInProviderJSON{ID: p.ID, Name: p.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// oidcFail ends a sign-in by sending the browser to the sign-in page with a reason, or a link
// to the profile.
func (s *server) oidcFail(w http.ResponseWriter, r *http.Request, event *store.RecordActivityParams, link bool, reason string, cause error) {
	if cause != nil {
		slog.WarnContext(r.Context(), "sign-in through a provider failed", "reason", reason, "error", cause)
	}
	event.Outcome = "rejected"
	if reason == "failed" {
		event.Outcome = "error"
	}
	// A link starts from the profile, so it ends there: the sign-in page sends a signed-in user away.
	page := "/login"
	if link {
		page = "/profile"
	}
	http.Redirect(w, r, page+"?error="+url.QueryEscape("oidc_"+reason), http.StatusSeeOther)
}

// oidcLogin sends the browser to the provider. With link=1 it links the identity to the
// signed-in user's account instead of signing in.
func (s *server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	w, event, record := s.trackLogin(w, r, true)
	defer record()
	p := s.oidcProvider(r.PathValue("id"))
	if p == nil {
		writeError(w, http.StatusNotFound, "no such provider")
		return
	}
	st := oidcState{
		Provider: p.ID,
		State:    rand.Text(),
		Nonce:    rand.Text(),
		Verifier: oauth2.GenerateVerifier(),
		Expires:  time.Now().Add(oidcLifetime).Unix(),
	}
	// A link starts from a session, which has no event of its own to record.
	link := r.URL.Query().Get("link") == "1"
	if link {
		event.Action = ""
		user, ok := s.sessionUser(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		st.LinkUser = user.ID
	}
	provider, err := p.discover(r.Context())
	if err != nil {
		s.oidcFail(w, r, event, link, "unavailable", err)
		return
	}
	value, err := s.sealState(st)
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.setStateCookie(w, value, int(oidcLifetime.Seconds()))
	conf := s.oauthConfig(p, provider)
	http.Redirect(w, r, conf.AuthCodeURL(st.State, oidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier)), http.StatusSeeOther)
}

func (s *server) oauthConfig(p *oidcProvider, provider *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  s.redirectURL(p),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
}

// flexBool reads "email_verified", which Google sends as a boolean and some providers as a string.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case bool:
		*b = flexBool(v)
	case string:
		*b = v == "true"
	}
	return nil
}

type oidcClaims struct {
	Email             string   `json:"email"`
	EmailVerified     flexBool `json:"email_verified"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
}

func (s *server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	w, event, record := s.trackLogin(w, r, true)
	defer record()
	p := s.oidcProvider(r.PathValue("id"))
	if p == nil {
		writeError(w, http.StatusNotFound, "no such provider")
		return
	}
	link := false
	cookie, err := r.Cookie(oidcCookie)
	s.setStateCookie(w, "", -1)
	if err != nil {
		s.oidcFail(w, r, event, link, "expired", nil)
		return
	}
	st, ok := s.openState(cookie.Value)
	link = ok && st.LinkUser != 0
	q := r.URL.Query()
	if !ok || st.Provider != p.ID || !hmac.Equal([]byte(st.State), []byte(q.Get("state"))) {
		s.oidcFail(w, r, event, link, "expired", nil)
		return
	}
	if q.Get("error") != "" || q.Get("code") == "" {
		s.oidcFail(w, r, event, link, "denied", nil)
		return
	}
	if st.LinkUser != 0 {
		event.Action = ""
	}

	provider, err := p.discover(r.Context())
	if err != nil {
		s.oidcFail(w, r, event, link, "unavailable", err)
		return
	}
	conf := s.oauthConfig(p, provider)
	token, err := conf.Exchange(r.Context(), q.Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		s.oidcFail(w, r, event, link, "denied", err)
		return
	}
	rawID, _ := token.Extra("id_token").(string)
	if rawID == "" {
		s.oidcFail(w, r, event, link, "denied", errors.New("no ID token"))
		return
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: p.ClientID}).Verify(r.Context(), rawID)
	if err != nil {
		s.oidcFail(w, r, event, link, "denied", err)
		return
	}
	if !hmac.Equal([]byte(idToken.Nonce), []byte(st.Nonce)) {
		s.oidcFail(w, r, event, link, "denied", errors.New("wrong nonce"))
		return
	}
	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		s.oidcFail(w, r, event, link, "denied", err)
		return
	}
	who := identity{issuer: p.Issuer, subject: idToken.Subject, claims: claims}

	if st.LinkUser != 0 {
		s.linkIdentity(w, r, event, st.LinkUser, who)
		return
	}
	user, reason, err := s.userForIdentity(r.Context(), who)
	if err != nil {
		s.oidcFail(w, r, event, link, "failed", err)
		return
	}
	if reason != "" {
		if user.ID != 0 {
			event.AccountID = &user.ID
		}
		s.oidcFail(w, r, event, link, reason, nil)
		return
	}
	if err := s.startSession(w, r, user.ID, event); err != nil {
		s.oidcFail(w, r, event, link, "failed", err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type identity struct {
	issuer, subject string
	claims          oidcClaims
}

func (i identity) verifiedEmail() string {
	if i.claims.EmailVerified {
		return strings.ToLower(strings.TrimSpace(i.claims.Email))
	}
	return ""
}

// userForIdentity finds the user an identity belongs to, or creates one when the settings
// allow. A non-empty reason says why no one may sign in.
func (s *server) userForIdentity(ctx context.Context, who identity) (store.User, string, error) {
	found, err := s.Queries.GetUserByIdentity(ctx, store.GetUserByIdentityParams{Issuer: who.issuer, Subject: who.subject})
	if err == nil {
		if found.User.Disabled {
			return found.User, "disabled", nil
		}
		if email := who.verifiedEmail(); email != "" {
			if err := s.Queries.SetIdentityEmail(ctx, store.SetIdentityEmailParams{Issuer: who.issuer, Subject: who.subject, Email: email}); err != nil {
				return store.User{}, "", err
			}
		}
		return found.User, "", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, "", err
	}

	settings, err := s.Queries.GetSignInSettings(ctx)
	if err != nil {
		return store.User{}, "", err
	}
	if !settings.CreateUsers {
		return store.User{}, "no_account", nil
	}
	if !admitted(settings.Allowed, who.verifiedEmail()) {
		return store.User{}, "not_allowed", nil
	}
	return s.createIdentityUser(ctx, who)
}

// admitted reports whether an address may create an account: any address when the list is
// empty, otherwise only a verified one that is on the list or in a domain on it.
func admitted(allowed []string, email string) bool {
	if len(allowed) == 0 {
		return true
	}
	if email == "" {
		return false
	}
	_, domain, _ := strings.Cut(email, "@")
	return slices.Contains(allowed, email) || slices.Contains(allowed, domain)
}

func (s *server) createIdentityUser(ctx context.Context, who identity) (store.User, string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.User{}, "", err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)

	login := ""
	for attempt := range 50 {
		candidate := suggestLogin(who.claims, attempt)
		if _, err := q.GetUserByLogin(ctx, candidate); errors.Is(err, pgx.ErrNoRows) {
			login = candidate
			break
		} else if err != nil {
			return store.User{}, "", err
		}
	}
	if login == "" {
		return store.User{}, "", errors.New("no free login")
	}
	user, err := q.CreateIdentityUser(ctx, store.CreateIdentityUserParams{Login: login, Email: who.verifiedEmail()})
	if err != nil {
		return store.User{}, "", err
	}
	err = q.CreateIdentity(ctx, store.CreateIdentityParams{
		UserID: user.ID, Issuer: who.issuer, Subject: who.subject, Email: who.verifiedEmail(),
	})
	if err != nil {
		return store.User{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.User{}, "", err
	}
	return user, "", nil
}

// suggestLogin makes a login from the claims: the preferred username, else the part of the
// email before the at sign, else the name, else a random one, reduced to what a login allows.
// A later attempt adds a number, then a random suffix, to the same base.
func suggestLogin(c oidcClaims, attempt int) string {
	email, _, _ := strings.Cut(c.Email, "@")
	base := ""
	for _, v := range []string{c.PreferredUsername, email, c.Name} {
		if base = cleanLogin(v); base != "" {
			break
		}
	}
	if base == "" {
		base = "user-" + strings.ToLower(rand.Text()[:8])
	}
	switch {
	case attempt == 0:
		return base
	case attempt < 10:
		return base + "-" + strconv.Itoa(attempt+1)
	default:
		return base + "-" + strings.ToLower(rand.Text()[:4])
	}
}

func cleanLogin(v string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(v) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 56 {
		out = out[:56]
	}
	if len(out) < 2 || auth.ValidateLogin(out) != nil {
		return ""
	}
	return out
}

// linkIdentity attaches an identity to the account of the user who started the link.
func (s *server) linkIdentity(w http.ResponseWriter, r *http.Request, event *store.RecordActivityParams, userID int64, who identity) {
	user, ok := s.sessionUser(r)
	if !ok || user.ID != userID {
		s.oidcFail(w, r, event, true, "expired", nil)
		return
	}
	existing, err := s.Queries.GetUserByIdentity(r.Context(), store.GetUserByIdentityParams{Issuer: who.issuer, Subject: who.subject})
	switch {
	case err == nil && existing.User.ID != user.ID:
		s.oidcFail(w, r, event, true, "taken", nil)
		return
	case errors.Is(err, pgx.ErrNoRows):
		err = s.Queries.CreateIdentity(r.Context(), store.CreateIdentityParams{
			UserID: user.ID, Issuer: who.issuer, Subject: who.subject, Email: who.verifiedEmail(),
		})
		if err != nil {
			s.oidcFail(w, r, event, true, "failed", err)
			return
		}
	case err != nil:
		s.oidcFail(w, r, event, true, "failed", err)
		return
	}
	http.Redirect(w, r, "/profile?linked=1", http.StatusSeeOther)
}

// sessionUser returns the user of the request's session without ending the request.
func (s *server) sessionUser(r *http.Request) (store.User, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return store.User{}, false
	}
	session, err := s.Queries.GetSessionUser(r.Context(), auth.HashSessionToken(cookie.Value))
	if err != nil {
		return store.User{}, false
	}
	return session.User, true
}

type identityJSON struct {
	ID        int64     `json:"id"`
	Issuer    string    `json:"issuer"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *server) listIdentities(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Queries.ListUserIdentities(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]identityJSON, 0, len(rows))
	for _, i := range rows {
		out = append(out, identityJSON{ID: i.ID, Issuer: i.Issuer, Email: i.Email, CreatedAt: i.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteIdentity unlinks a provider. A user with no password and no other provider could
// never sign in again, so the last way in stays.
func (s *server) deleteIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	user := currentUser(r)
	rows, err := s.Queries.ListUserIdentities(r.Context(), user.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if user.PasswordHash == "" && len(rows) <= 1 {
		writeError(w, http.StatusConflict, "this is the only way to sign in to the account")
		return
	}
	n, err := s.Queries.DeleteUserIdentity(r.Context(), store.DeleteUserIdentityParams{ID: id, UserID: user.ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "no such identity")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type signInSettingsJSON struct {
	CreateUsers        bool     `json:"createUsers"`
	Allowed            []string `json:"allowed"`
	PasswordAdminsOnly bool     `json:"passwordAdminsOnly"`
}

func (s *server) getSignInSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Queries.GetSignInSettings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toSignInSettingsJSON(settings))
}

func toSignInSettingsJSON(s store.SignInSetting) signInSettingsJSON {
	return signInSettingsJSON{CreateUsers: s.CreateUsers, Allowed: s.Allowed, PasswordAdminsOnly: s.PasswordAdminsOnly}
}

const maxAllowedEntries = 200

func (s *server) updateSignInSettings(w http.ResponseWriter, r *http.Request) {
	var req signInSettingsJSON
	if !readJSON(w, r, &req) {
		return
	}
	allowed := []string{}
	for _, entry := range req.Allowed {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" || slices.Contains(allowed, entry) {
			continue
		}
		if strings.ContainsAny(entry, " \t,;") || strings.HasPrefix(entry, "@") || len(entry) > 254 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is not an email address or a domain", entry))
			return
		}
		allowed = append(allowed, entry)
	}
	if len(allowed) > maxAllowedEntries {
		writeError(w, http.StatusBadRequest, "the list is too long")
		return
	}
	settings, err := s.Queries.UpdateSignInSettings(r.Context(), store.UpdateSignInSettingsParams{
		CreateUsers: req.CreateUsers, Allowed: allowed, PasswordAdminsOnly: req.PasswordAdminsOnly,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toSignInSettingsJSON(settings))
}
