// Package middleware holds cross-cutting HTTP middleware: session
// authentication (Authenticate) and role-based authorization
// (RequireRole). (requestLogger stays in cmd/api — it's the one
// middleware simple enough not to need its own package.)
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"noxoj/internal/auth"
)

type contextKey string

const (
	userIDContextKey contextKey = "userID"
	rolesContextKey  contextKey = "roles"
)

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
// authenticated user's ID and roles (read straight from the token's
// claims — no database lookup) in the request context for handlers
// downstream. Missing or invalid tokens get a 401 and the request
// never reaches the handler.
func Authenticate(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(AccessTokenCookieName)
			if err != nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ParseAccessToken(cookie.Value, secret)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired session"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
			ctx = context.WithValue(ctx, rolesContextKey, claims.Roles)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole must run after Authenticate (it reads what Authenticate
// stores). A request from a user who doesn't hold roleName gets 403
// Forbidden — deliberately different from Authenticate's 401: this is
// "I know exactly who you are, and the answer is no," not "I don't
// know who you are."
func RequireRole(roleName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			roles, ok := RolesFromContext(r.Context())
			if !ok {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}
			for _, role := range roles {
				if role == roleName {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
		})
	}
}

// UserIDFromContext retrieves the authenticated user's ID, set by
// Authenticate. ok is false if called outside an authenticated route.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDContextKey).(uuid.UUID)
	return id, ok
}

// RolesFromContext retrieves the authenticated user's roles, set by
// Authenticate. ok is false if called outside an authenticated route.
func RolesFromContext(ctx context.Context) ([]string, bool) {
	roles, ok := ctx.Value(rolesContextKey).([]string)
	return roles, ok
}
