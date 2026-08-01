package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"noxoj/internal/auth"
)

func protectedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "no user ID in context", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(userID.String()))
	})
}

func tokenWithRoles(t *testing.T, secret []byte, userID uuid.UUID, roles []string) string {
	t.Helper()
	token, err := auth.GenerateAccessToken(userID, roles, secret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	return token
}

func TestAuthenticate_ValidToken(t *testing.T) {
	secret := []byte("test-secret")
	userID := uuid.New()
	token := tokenWithRoles(t, secret, userID, nil)

	handler := Authenticate(secret)(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if rec.Body.String() != userID.String() {
		t.Errorf("expected body %q, got %q", userID.String(), rec.Body.String())
	}
}

func TestAuthenticate_MissingCookie(t *testing.T) {
	handler := Authenticate([]byte("test-secret"))(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	handler := Authenticate([]byte("test-secret"))(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: "not-a-real-token"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestAuthenticate_WrongSecret(t *testing.T) {
	token := tokenWithRoles(t, []byte("secret-a"), uuid.New(), nil)

	handler := Authenticate([]byte("secret-b"))(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func adminOnlyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("admin area"))
	})
}

func TestRequireRole_AllowsMatchingRole(t *testing.T) {
	secret := []byte("test-secret")
	token := tokenWithRoles(t, secret, uuid.New(), []string{"contestant", "admin"})

	handler := Authenticate(secret)(RequireRole("admin")(adminOnlyHandler()))

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestRequireRole_RejectsMissingRole(t *testing.T) {
	secret := []byte("test-secret")
	token := tokenWithRoles(t, secret, uuid.New(), []string{"contestant"})

	handler := Authenticate(secret)(RequireRole("admin")(adminOnlyHandler()))

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

func TestRequireRole_UnauthenticatedGets401NotForbidden(t *testing.T) {
	handler := Authenticate([]byte("test-secret"))(RequireRole("admin")(adminOnlyHandler()))

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// No cookie at all -> Authenticate itself rejects with 401 before
	// RequireRole ever runs. Confirms the two failure modes stay
	// distinct: missing identity is 401, insufficient permission is 403.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}
