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

func TestAuthenticate_ValidToken(t *testing.T) {
	secret := []byte("test-secret")
	userID := uuid.New()
	token, err := auth.GenerateAccessToken(userID, secret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

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
	token, err := auth.GenerateAccessToken(uuid.New(), []byte("secret-a"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler := Authenticate([]byte("secret-b"))(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}
