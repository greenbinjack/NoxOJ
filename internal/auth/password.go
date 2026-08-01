// Package auth handles password hashing and verification.
package auth

import "golang.org/x/crypto/bcrypt"

// bcryptCost trades hashing speed for brute-force resistance. The
// default (10) is a reasonable floor; going higher slows down both
// attackers and legitimate logins, so this isn't "as high as
// possible" — it's a deliberate balance, revisited if hardware
// makes the default too fast to be safe.
const bcryptCost = 12

// HashPassword returns a salted bcrypt hash of the given password.
// bcrypt generates and embeds a random salt automatically — callers
// never handle salts themselves.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the given bcrypt
// hash. Returns a non-nil error on mismatch — callers should treat
// any error here as "invalid credentials," not inspect it further.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// dummyHash is a real, validly-formatted bcrypt hash of a fixed,
// meaningless string — computed once at startup, not hardcoded as a
// magic string. It exists purely so a login attempt against a
// username that doesn't exist can still run a full bcrypt comparison
// (see CheckPassword) instead of returning instantly. Without this,
// "no such user" would respond in microseconds while "wrong
// password" takes bcrypt's ~100ms, letting an attacker enumerate
// valid usernames purely by timing responses.
var dummyHash = func() string {
	hash, err := HashPassword("dummy-password-used-only-for-timing-safety")
	if err != nil {
		panic("auth: failed to compute dummy hash: " + err.Error())
	}
	return hash
}()

// DummyHash returns a real bcrypt hash suitable for a timing-safe
// comparison when no real user/hash exists to compare against.
func DummyHash() string {
	return dummyHash
}
