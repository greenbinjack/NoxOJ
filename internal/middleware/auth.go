// Package middleware holds cross-cutting HTTP middleware — currently
// just auth. (requestLogger stays in cmd/api for now, since it's the
// only handler that doesn't need its own package yet; this one does,
// because RBAC middleware in Sprint 11 will live alongside it.)
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"noxoj/internal/auth"
)

type contextKey string

const userIDContextKey contextKey = "userID"

// AccessTokenCookieName is the cookie the access token is stored in.
// Exported so the handler that sets it and the middleware that reads
// it agree on the name without either hardcoding a shared string.
const AccessTokenCookieName = "access_token"

// RefreshTokenCookieName is the cookie the refresh token is stored
// in. Only ever read by the /refresh and /logout handlers, never by
// this package's own Authenticate middleware — the refresh token is
// deliberately not a substitute for a valid access token on ordinary
// requests.
const RefreshTokenCookieName = "refresh_token"

// Authenticate requires a valid access token cookie, storing the
// authenticated user's ID in the request context for handlers
// downstream to read via UserIDFromContext. Missing or invalid
// tokens get a 401 and the request never reaches the handler.
func Authenticate(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(AccessTokenCookieName)
			if err != nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			userID, err := auth.ParseAccessToken(cookie.Value, secret)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired session"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext retrieves the authenticated user's ID, set by
// Authenticate. ok is false if called outside an authenticated route.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDContextKey).(uuid.UUID)
	return id, ok
}
