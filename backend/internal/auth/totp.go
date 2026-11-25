package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	// TOTPIssuer is the name authenticator apps show for the account.
	TOTPIssuer = "Magellanica Photos"

	totpPeriod = 30
	// totpSkew is how many steps of clock drift either way a code may have.
	totpSkew = 1

	// RecoveryCodeCount is how many recovery codes a user gets.
	RecoveryCodeCount = 10

	// PendingLifetime is how long the token between the password and the code stays valid.
	PendingLifetime = 5 * time.Minute

	recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	recoveryHalf     = 5
)

// TOTPKey encrypts the secrets of authenticators and signs the tokens between the two steps
// of signing in. It is made from the TOTP_KEY setting; the zero value is off.
type TOTPKey struct {
	aead cipher.AEAD
	mac  []byte
}

// NewTOTPKey derives the encryption and signing keys from the setting. An empty setting gives
// a key that is off.
func NewTOTPKey(setting string) (TOTPKey, error) {
	if setting == "" {
		return TOTPKey{}, nil
	}
	enc := sha256.Sum256([]byte("magellanica totp encryption\x00" + setting))
	block, err := aes.NewCipher(enc[:])
	if err != nil {
		return TOTPKey{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return TOTPKey{}, err
	}
	mac := sha256.Sum256([]byte("magellanica totp pending token\x00" + setting))
	return TOTPKey{aead: aead, mac: mac[:]}, nil
}

// On reports whether two-factor sign-in can be used.
func (k TOTPKey) On() bool { return k.aead != nil }

// Encrypt seals a secret. The user's id is bound to it, so a secret copied between rows does not open.
func (k TOTPKey) Encrypt(userID int64, secret string) ([]byte, error) {
	if !k.On() {
		return nil, errors.New("TOTP_KEY is not set")
	}
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return k.aead.Seal(nonce, nonce, []byte(secret), userAAD(userID)), nil
}

// Decrypt opens a secret sealed by Encrypt.
func (k TOTPKey) Decrypt(userID int64, sealed []byte) (string, error) {
	if !k.On() {
		return "", errors.New("TOTP_KEY is not set")
	}
	n := k.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("sealed secret is too short")
	}
	plain, err := k.aead.Open(nil, sealed[:n], sealed[n:], userAAD(userID))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func userAAD(userID int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(userID))
}

// NewTOTPSecret makes a secret for a user, with the address an authenticator app imports.
func NewTOTPSecret(login string) (secret, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      TOTPIssuer,
		AccountName: login,
		Period:      totpPeriod,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// CheckTOTP reports the time step a code is valid for, or false. The caller accepts a step
// only if it is greater than the last one it accepted, so a code works once.
func CheckTOTP(secret, code string, now time.Time) (step int64, ok bool) {
	code = strings.NewReplacer(" ", "", "-", "").Replace(code)
	if len(code) != 6 {
		return 0, false
	}
	current := now.Unix() / totpPeriod
	// Every candidate is compared, so the time taken does not tell which step matched.
	for offset := int64(-totpSkew); offset <= totpSkew; offset++ {
		want, err := totp.GenerateCodeCustom(secret, time.Unix((current+offset)*totpPeriod, 0), totp.ValidateOpts{
			Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
		})
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			step, ok = current+offset, true
		}
	}
	return step, ok
}

// NewRecoveryCodes returns codes to show once, and their hashes to keep.
func NewRecoveryCodes() (codes []string, hashes [][]byte, err error) {
	for range RecoveryCodeCount {
		raw := make([]byte, 2*recoveryHalf)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		var b strings.Builder
		for i, v := range raw {
			if i == recoveryHalf {
				b.WriteByte('-')
			}
			b.WriteByte(recoveryAlphabet[int(v)%len(recoveryAlphabet)])
		}
		codes = append(codes, b.String())
		hashes = append(hashes, HashRecoveryCode(b.String()))
	}
	return codes, hashes, nil
}

// LooksLikeRecoveryCode tells a recovery code from a six-digit one.
func LooksLikeRecoveryCode(code string) bool {
	return len(normalizeRecoveryCode(code)) == 2*recoveryHalf
}

// HashRecoveryCode returns the hash a recovery code is stored as.
func HashRecoveryCode(code string) []byte {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return sum[:]
}

func normalizeRecoveryCode(code string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(code))
}

// NewPending signs a token that says the user gave the right password and has five minutes
// to give the code.
func (k TOTPKey) NewPending(userID int64, now time.Time) string {
	payload := binary.BigEndian.AppendUint64(nil, uint64(userID))
	payload = binary.BigEndian.AppendUint64(payload, uint64(now.Add(PendingLifetime).Unix()))
	return base64.RawURLEncoding.EncodeToString(payload) + "." + hex.EncodeToString(k.sign(payload))
}

// CheckPending returns the user a pending token was made for.
func (k TOTPKey) CheckPending(token string, now time.Time) (int64, bool) {
	if !k.On() {
		return 0, false
	}
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil || len(payload) != 16 {
		return 0, false
	}
	got, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(got, k.sign(payload)) {
		return 0, false
	}
	if now.Unix() >= int64(binary.BigEndian.Uint64(payload[8:])) {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(payload[:8])), true
}

func (k TOTPKey) sign(payload []byte) []byte {
	m := hmac.New(sha256.New, k.mac)
	m.Write(payload)
	return m.Sum(nil)
}
