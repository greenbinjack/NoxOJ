package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"noxoj/internal/auth"
	"noxoj/internal/domain"
	authmw "noxoj/internal/middleware"
)

var (
	testJWTSecret  = []byte("test-secret-for-main-tests")
	testCORSOrigin = "http://localhost:5173"
)

// stubHandler stands in for real handlers in tests that have nothing
// to do with what that handler does — they shouldn't need a live
// database connection just to construct a router. The real behavior
// of Register/Login/Refresh/Logout/Me is tested where they actually
// live, in internal/handler — these tests only cover routing/wiring.
func stubHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

var stubHandlers = Handlers{
	Register:             stubHandler,
	Login:                stubHandler,
	Refresh:              stubHandler,
	Logout:               stubHandler,
	Me:                   stubHandler,
	RequestPasswordReset: stubHandler,
	ConfirmPasswordReset: stubHandler,
	DeactivateUser:       stubHandler,
}

func TestRootRoute(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

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
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestReadyzRoute(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestReadyzRoute_FailsWhenADependencyIsDown(t *testing.T) {
	failingCheck := func() error { return errors.New("database unreachable") }
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers, failingCheck)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestUsersMeRoute_RequiresAuthentication(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d without a token, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestUsersMeRoute_ReachesHandlerWhenAuthenticated(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	token, err := auth.GenerateAccessToken(uuid.New(), []string{domain.RoleContestant}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	req.AddCookie(&http.Cookie{Name: authmw.AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// stubHandler always returns 501 — reaching it (not a 401 from
	// Authenticate) is what proves the route is wired correctly.
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected the request to reach the handler (%d), got %d", http.StatusNotImplemented, rec.Code)
	}
}

func TestPasswordResetRequestRoute(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	req := httptest.NewRequest(http.MethodPost, "/password-reset/request", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// stubHandler always returns 501 — reaching it proves the route is
	// wired, unauthenticated (a forgotten password means no valid
	// session to authenticate with).
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected the request to reach the handler (%d), got %d", http.StatusNotImplemented, rec.Code)
	}
}

func TestPasswordResetConfirmRoute(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	req := httptest.NewRequest(http.MethodPost, "/password-reset/confirm", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected the request to reach the handler (%d), got %d", http.StatusNotImplemented, rec.Code)
	}
}

func TestAdminPingRoute_RequiresAdminRole(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	token, err := auth.GenerateAccessToken(uuid.New(), []string{domain.RoleContestant}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.AddCookie(&http.Cookie{Name: authmw.AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected a plain contestant to get %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

func TestAdminDeactivateRoute_RequiresAdminRole(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	token, err := auth.GenerateAccessToken(uuid.New(), []string{domain.RoleContestant}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uuid.New().String()+"/deactivate", nil)
	req.AddCookie(&http.Cookie{Name: authmw.AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected a plain contestant to get %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

func TestAdminDeactivateRoute_ReachesHandlerForAdmin(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	token, err := auth.GenerateAccessToken(uuid.New(), []string{domain.RoleAdmin}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uuid.New().String()+"/deactivate", nil)
	req.AddCookie(&http.Cookie{Name: authmw.AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// stubHandler always returns 501 — reaching it proves the route
	// (including its {id} URL parameter) is wired correctly for an
	// admin.
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected the request to reach the handler (%d), got %d", http.StatusNotImplemented, rec.Code)
	}
}

func TestAdminPingRoute_AllowsAdmin(t *testing.T) {
	r := newRouter(zerolog.Nop(), testJWTSecret, testCORSOrigin, stubHandlers)

	token, err := auth.GenerateAccessToken(uuid.New(), []string{domain.RoleContestant, domain.RoleAdmin}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.AddCookie(&http.Cookie{Name: authmw.AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected an admin to get %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}
