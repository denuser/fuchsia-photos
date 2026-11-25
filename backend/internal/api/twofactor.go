package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image/png"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

// twoFactorKey is the key of a user in the limiter of the code step. It is per user, so guessing
// codes for one account is slowed down whichever login spelling is used.
func twoFactorKey(userID int64) string {
	return "totp:" + strconv.FormatInt(userID, 10)
}

// tooManyCodes answers when the code step of a user is waiting out its delay.
func (s *server) tooManyCodes(w http.ResponseWriter, userID int64) bool {
	ok, wait := s.codes.Allow(twoFactorKey(userID))
	if ok {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
	return true
}

// checkSecondFactor accepts a code from the authenticator, once per time step, or a recovery
// code, once. It records a wrong code with the limiter.
func (s *server) checkSecondFactor(ctx context.Context, user store.User, code string) (bool, error) {
	ok, err := s.acceptCode(ctx, user, code)
	if err != nil {
		return false, err
	}
	if !ok {
		s.codes.Fail(twoFactorKey(user.ID))
		return false, nil
	}
	s.codes.Reset(twoFactorKey(user.ID))
	return true, nil
}

func (s *server) acceptCode(ctx context.Context, user store.User, code string) (bool, error) {
	if auth.LooksLikeRecoveryCode(code) {
		n, err := s.Queries.UseRecoveryCode(ctx, store.UseRecoveryCodeParams{UserID: user.ID, CodeHash: auth.HashRecoveryCode(code)})
		return n == 1, err
	}
	secret, err := s.TOTP.Decrypt(user.ID, user.TotpSecret)
	if err != nil {
		return false, err
	}
	step, ok := auth.CheckTOTP(secret, code, time.Now())
	if !ok {
		return false, nil
	}
	n, err := s.Queries.ClaimTOTPStep(ctx, store.ClaimTOTPStepParams{ID: user.ID, Step: step})
	return n == 1, err
}

// loginTOTP is the second step of signing in: the token the password step returned and a code.
func (s *server) loginTOTP(w http.ResponseWriter, r *http.Request) {
	w, event, record := s.trackLogin(w, r, !s.maintenanceLogin(r))
	defer record()
	var req struct {
		Token string `json:"token"`
		Code  string `json:"code"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	userID, ok := s.TOTP.CheckPending(req.Token, time.Now())
	if !ok {
		writeError(w, http.StatusUnauthorized, "the sign-in has expired, enter the password again")
		return
	}
	event.AccountID = &userID
	if s.tooManyCodes(w, userID) {
		return
	}
	user, err := s.Queries.GetUser(r.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "the sign-in has expired, enter the password again")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if user.Disabled || !user.TotpEnabled {
		writeError(w, http.StatusUnauthorized, "the sign-in has expired, enter the password again")
		return
	}
	ok, err = s.checkSecondFactor(r.Context(), user, req.Code)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "wrong code")
		return
	}
	if err := s.startSession(w, r, user.ID, event); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserJSON(user))
}

type twoFactorJSON struct {
	// Available is false when the server has no TOTP_KEY, so two-factor sign-in cannot be turned on.
	Available bool `json:"available"`
	Enabled   bool `json:"enabled"`
	// RecoveryCodesLeft counts the recovery codes not used yet.
	RecoveryCodesLeft int64 `json:"recoveryCodesLeft"`
}

func (s *server) getTwoFactor(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	out := twoFactorJSON{Available: s.TOTP.On(), Enabled: user.TotpEnabled}
	if user.TotpEnabled {
		var err error
		if out.RecoveryCodesLeft, err = s.Queries.CountRecoveryCodes(r.Context(), user.ID); err != nil {
			internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// setupTwoFactor makes a secret and shows it as a picture and as text. Nothing changes for
// sign-in until enableTwoFactor gets a correct code.
func (s *server) setupTwoFactor(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !s.TOTP.On() {
		writeError(w, http.StatusConflict, "two-factor sign-in is not set up on this server: TOTP_KEY is missing")
		return
	}
	if user.PasswordHash == "" {
		writeError(w, http.StatusConflict, "set a password first: two-factor sign-in protects the password")
		return
	}
	if user.TotpEnabled {
		writeError(w, http.StatusConflict, "two-factor sign-in is already on")
		return
	}
	secret, uri, err := auth.NewTOTPSecret(user.Login)
	if err != nil {
		internalError(w, r, err)
		return
	}
	sealed, err := s.TOTP.Encrypt(user.ID, secret)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if _, err := s.Queries.StartTOTPSetup(r.Context(), store.StartTOTPSetupParams{ID: user.ID, Secret: sealed}); err != nil {
		internalError(w, r, err)
		return
	}
	qr, err := qrDataURI(uri)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Secret string `json:"secret"`
		URI    string `json:"uri"`
		QR     string `json:"qr"`
	}{secret, uri, qr})
}

func qrDataURI(uri string) (string, error) {
	key, err := otp.NewKeyFromURL(uri)
	if err != nil {
		return "", err
	}
	img, err := key.Image(224, 224)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// enableTwoFactor turns two-factor sign-in on once the app shows the right code, and returns
// the recovery codes, which are shown only now.
func (s *server) enableTwoFactor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	user := currentUser(r)
	if user.TotpEnabled || len(user.TotpSecret) == 0 {
		writeError(w, http.StatusConflict, "start the setup first")
		return
	}
	if s.tooManyCodes(w, user.ID) {
		return
	}
	secret, err := s.TOTP.Decrypt(user.ID, user.TotpSecret)
	if err != nil {
		internalError(w, r, err)
		return
	}
	step, ok := auth.CheckTOTP(secret, req.Code, time.Now())
	if !ok {
		s.codes.Fail(twoFactorKey(user.ID))
		writeError(w, http.StatusUnauthorized, "wrong code")
		return
	}
	s.codes.Reset(twoFactorKey(user.ID))
	codes, hashes, err := auth.NewRecoveryCodes()
	if err != nil {
		internalError(w, r, err)
		return
	}

	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := s.Queries.WithTx(tx)
	n, err := q.EnableTOTP(r.Context(), store.EnableTOTPParams{ID: user.ID, Step: step})
	if err == nil && n == 1 {
		if err = q.DeleteRecoveryCodes(r.Context(), user.ID); err == nil {
			err = q.InsertRecoveryCodes(r.Context(), store.InsertRecoveryCodesParams{UserID: user.ID, Hashes: hashes})
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if n != 1 {
		writeError(w, http.StatusConflict, "two-factor sign-in is already on")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}{codes})
}

// disableTwoFactor asks for the password and a code, so a stolen session cannot turn it off.
func (s *server) disableTwoFactor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	user := currentUser(r)
	if !user.TotpEnabled {
		writeError(w, http.StatusConflict, "two-factor sign-in is off")
		return
	}
	if s.tooManyCodes(w, user.ID) {
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		s.codes.Fail(twoFactorKey(user.ID))
		writeError(w, http.StatusForbidden, "the current password is wrong")
		return
	}
	ok, err := s.checkSecondFactor(r.Context(), user, req.Code)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "wrong code")
		return
	}
	if err := s.clearTwoFactor(r.Context(), user.ID); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clearTwoFactor forgets the secret and the recovery codes of a user.
func (s *server) clearTwoFactor(ctx context.Context, userID int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	if err := q.ClearTOTP(ctx, userID); err != nil {
		return err
	}
	if err := q.DeleteRecoveryCodes(ctx, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DisableUnreadableTwoFactor runs at start. A secret that the current TOTP_KEY cannot open, as
// after a restore on another machine without the key, would lock its user out, so two-factor
// sign-in is turned off for those users and the warning names them.
func DisableUnreadableTwoFactor(ctx context.Context, q *store.Queries, key auth.TOTPKey) error {
	users, err := q.ListTOTPUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if key.On() {
			if _, err := key.Decrypt(u.ID, u.TotpSecret); err == nil {
				continue
			}
		}
		if err := q.ClearTOTP(ctx, u.ID); err != nil {
			return err
		}
		if err := q.DeleteRecoveryCodes(ctx, u.ID); err != nil {
			return err
		}
		slog.WarnContext(ctx, "two-factor sign-in turned off: TOTP_KEY is missing or is not the key the secret was made with; the user has to set it up again", "login", u.Login)
	}
	return nil
}
