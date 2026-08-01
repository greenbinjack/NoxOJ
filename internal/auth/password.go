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
