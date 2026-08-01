package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessTokenTTL is deliberately short — if a token leaks, this is
// the entire window of exposure. Sprint 10 adds a longer-lived
// refresh token so short-lived access tokens don't mean re-entering
// a password every 15 minutes forever.
const AccessTokenTTL = 15 * time.Minute

var ErrInvalidToken = errors.New("invalid or expired token")

type claims struct {
	jwt.RegisteredClaims
}

// GenerateAccessToken issues a signed JWT identifying userID, valid
// for AccessTokenTTL.
func GenerateAccessToken(userID uuid.UUID, secret []byte) (string, error) {
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString(secret)
}

// ParseAccessToken verifies tokenString's signature and expiry and
// returns the user ID it identifies.
//
// The explicit check that the token's algorithm is HMAC is not
// decoration — it's the fix for a real, well-known JWT vulnerability
// class ("algorithm confusion"), where a library that trusts
// whatever algorithm the token *claims* to use can be tricked into
// verifying a forged token against the wrong kind of key (or, in the
// worst case, accepting alg="none" entirely). We only ever accept
// what we ourselves signed with.
func ParseAccessToken(tokenString string, secret []byte) (uuid.UUID, error) {
	c := &claims{}
	token, err := jwt.ParseWithClaims(tokenString, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return uuid.Nil, ErrInvalidToken
	}

	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	return userID, nil
}
