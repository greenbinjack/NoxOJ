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
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

// TokenClaims is what ParseAccessToken hands back — everything a
// caller needs to authenticate *and* authorize a request without a
// second lookup.
type TokenClaims struct {
	UserID uuid.UUID
	Roles  []string
}

// GenerateAccessToken issues a signed JWT identifying userID and the
// roles they held at the moment of issuance, valid for AccessTokenTTL.
// Roles are a snapshot, not a live value — a promotion or demotion
// takes effect on the user's next login or token refresh (at most
// AccessTokenTTL later), not instantly. That's a deliberate tradeoff:
// the alternative is a database lookup on every authorized request,
// which defeats the point of a stateless token in the first place.
func GenerateAccessToken(userID uuid.UUID, roles []string, secret []byte) (string, error) {
	c := claims{
		Roles: roles,
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
// returns the identity and roles it carries.
//
// The explicit check that the token's algorithm is HMAC is not
// decoration — it's the fix for a real, well-known JWT vulnerability
// class ("algorithm confusion"), where a library that trusts
// whatever algorithm the token *claims* to use can be tricked into
// verifying a forged token against the wrong kind of key (or, in the
// worst case, accepting alg="none" entirely). We only ever accept
// what we ourselves signed with.
func ParseAccessToken(tokenString string, secret []byte) (TokenClaims, error) {
	c := &claims{}
	token, err := jwt.ParseWithClaims(tokenString, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return TokenClaims{}, ErrInvalidToken
	}

	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return TokenClaims{}, ErrInvalidToken
	}
	return TokenClaims{UserID: userID, Roles: c.Roles}, nil
}
