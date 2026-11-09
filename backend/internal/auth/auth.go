// Package auth implements password hashing and session tokens.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemoryKiB = 64 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLen    = 32
	saltLen        = 16

	minPasswordLen = 8
	maxPasswordLen = 256
)

// Roles.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

var (
	loginPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{2,64}$`)

	// Each hash takes 64 MiB and a core, so concurrent logins must not exhaust the NAS.
	hashSlots = make(chan struct{}, 2)
)

// ValidateLogin reports whether login is acceptable.
func ValidateLogin(login string) error {
	if !loginPattern.MatchString(login) {
		return errors.New("a login must be 2 to 64 characters: letters, digits, dot, dash or underscore")
	}
	return nil
}

// ValidatePassword reports whether password is acceptable.
func ValidatePassword(password string) error {
	if len(password) < minPasswordLen || len(password) > maxPasswordLen {
		return fmt.Errorf("a password must be %d to %d characters", minPasswordLen, maxPasswordLen)
	}
	return nil
}

// HashPassword returns an encoded argon2id hash of password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := derive(password, salt, argonMemoryKiB, argonTime, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches an encoded hash.
func CheckPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := derive(password, salt, memory, time, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func derive(password string, salt []byte, memory, time uint32, threads uint8, keyLen uint32) []byte {
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()
	return argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
}

// NewSessionToken returns a random token for the cookie and the hash to store.
// Only the hash is stored, so a database leak does not expose live sessions.
func NewSessionToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashSessionToken(token), nil
}

// HashSessionToken returns the stored form of a session token.
func HashSessionToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// appPasswordAlphabet is Crockford's base32 in lower case. It leaves out i, l, o
// and u, so a password typed on a phone has no characters that look alike. Its
// 32 characters divide 256 evenly, so each is equally likely.
const appPasswordAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// NewAppPassword returns a random password for a WebDAV client, in four groups
// of five characters, and the hash to store. With 100 bits of randomness it
// needs no slow hash: it cannot be guessed, unlike a password a person picks.
func NewAppPassword() (password string, hash []byte, err error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	var b strings.Builder
	for i, v := range raw {
		if i > 0 && i%5 == 0 {
			b.WriteByte('-')
		}
		b.WriteByte(appPasswordAlphabet[int(v)%len(appPasswordAlphabet)])
	}
	return b.String(), HashAppPassword(b.String()), nil
}

// HashAppPassword returns the stored form of an app password.
func HashAppPassword(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return sum[:]
}
