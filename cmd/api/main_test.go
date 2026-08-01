package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"noxoj/internal/auth"
	authmw "noxoj/internal/middleware"
)

var testJWTSecret = []byte("test-secret-for-main-tests")

// stubHandler stands in for real handlers in tests that have nothing
// to do with what that handler does — they shouldn't need a live
// database connection just to construct a router.
func stubHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

var stubHandlers = Handlers{
	Register: stubHandler,
	Login:    stubHandler,
	Refresh:  stubHandler,
	Logout:   stubHandler,
}

func TestRootRoute(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, stubHandlers)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	want := "NoxOJ API — Sprint 1 skeleton is alive"
	if got := rec.Body.String(); got != want {
		t.Fatalf("expected body %q, got %q", want, got)
	}
}

func TestHealthzRoute(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, stubHandlers)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestReadyzRoute(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, stubHandlers)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestReadyzRoute_FailsWhenADependencyIsDown(t *testing.T) {
	failingCheck := func() error { return errors.New("database unreachable") }
	r := newRouter(zerolog.Nop(), testJWTSecret, stubHandlers, failingCheck)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestMeRoute_RequiresAuthentication(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, stubHandlers)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d without a token, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestMeRoute_ReturnsAuthenticatedUserID(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, stubHandlers)

	userID := uuid.New()
	token, err := auth.GenerateAccessToken(userID, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: authmw.AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	want := `{"user_id":"` + userID.String() + `"}`
	if got := rec.Body.String(); got != want {
		t.Fatalf("expected body %q, got %q", want, got)
	}
}
