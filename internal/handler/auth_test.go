package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"noxoj/internal/config"
	authmw "noxoj/internal/middleware"
	"noxoj/internal/ratelimit"
	"noxoj/internal/repository"
	"noxoj/internal/tokenstore"
)

func testAuthHandler(t *testing.T) (*AuthHandler, *UserHandler) {
	t.Helper()
	db := testHandlerDB(t)

	redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	t.Cleanup(func() { redisClient.Close() })

	users := repository.NewUserRepository(db)
	limiter := ratelimit.NewLoginLimiter(5, 15*time.Minute)
	refreshTokens := tokenstore.NewRefreshTokenStore(redisClient)

	auth := NewAuthHandler(testLoggerNop(), users, testJWTSecret, limiter, refreshTokens, config.Development)
	user := NewUserHandler(testLoggerNop(), users)
	return auth, user
}

func doLogin(t *testing.T, h *AuthHandler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, h.Login, "/login", body)
}

func cookieFrom(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func registerAndLogin(t *testing.T, authH *AuthHandler, userH *UserHandler, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	reg := doRegister(t, userH, map[string]any{"username": username, "password": password, "display_name": "Auth Test"})
	if reg.Code != http.StatusCreated {
		t.Fatalf("setup: expected registration to succeed, got %d: %s", reg.Code, reg.Body.String())
	}
	return doLogin(t, authH, map[string]any{"username": username, "password": password})
}

func TestLogin_Success(t *testing.T) {
	authH, userH := testAuthHandler(t)
	db := testHandlerDB(t)
	username := "sprint10_login_ok"
	password := "correct-horse-battery"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	rec := registerAndLogin(t, authH, userH, username, password)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	access := cookieFrom(rec, authmw.AccessTokenCookieName)
	refresh := cookieFrom(rec, authmw.RefreshTokenCookieName)
	if access == nil || access.Value == "" {
		t.Error("expected an access_token cookie")
	}
	if refresh == nil || refresh.Value == "" {
		t.Error("expected a refresh_token cookie")
	}
	if access != nil && !access.HttpOnly {
		t.Error("expected access_token to be HttpOnly")
	}
	if refresh != nil && !refresh.HttpOnly {
		t.Error("expected refresh_token to be HttpOnly")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	authH, userH := testAuthHandler(t)
	db := testHandlerDB(t)
	username := "sprint10_login_wrongpw"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	doRegister(t, userH, map[string]any{"username": username, "password": "correct-horse-battery", "display_name": "X"})
	rec := doLogin(t, authH, map[string]any{"username": username, "password": "totally-wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
}

func TestLogin_LockedOutAfterRepeatedFailures(t *testing.T) {
	authH, userH := testAuthHandler(t)
	db := testHandlerDB(t)
	username := "sprint10_login_lockout"
	password := "correct-horse-battery"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	doRegister(t, userH, map[string]any{"username": username, "password": password, "display_name": "X"})

	for i := 0; i < 5; i++ {
		rec := doLogin(t, authH, map[string]any{"username": username, "password": "wrong"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected %d, got %d", i+1, http.StatusUnauthorized, rec.Code)
		}
	}

	rec := doLogin(t, authH, map[string]any{"username": username, "password": password})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected %d after repeated failures, got %d: %s", http.StatusTooManyRequests, rec.Code, rec.Body.String())
	}
}

func TestRefresh_Success(t *testing.T) {
	authH, userH := testAuthHandler(t)
	db := testHandlerDB(t)
	username := "sprint10_refresh_ok"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	loginRec := registerAndLogin(t, authH, userH, username, "correct-horse-battery")
	refreshCookie := cookieFrom(loginRec, authmw.RefreshTokenCookieName)
	if refreshCookie == nil {
		t.Fatal("setup: expected a refresh_token cookie from login")
	}

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.AddCookie(refreshCookie)
	rec := httptest.NewRecorder()
	authH.Refresh(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	newAccess := cookieFrom(rec, authmw.AccessTokenCookieName)
	newRefresh := cookieFrom(rec, authmw.RefreshTokenCookieName)
	if newAccess == nil || newAccess.Value == "" {
		t.Error("expected a new access_token cookie")
	}
	if newRefresh == nil || newRefresh.Value == "" {
		t.Error("expected a new refresh_token cookie")
	}
	if newRefresh != nil && newRefresh.Value == refreshCookie.Value {
		t.Error("expected the refresh token to rotate — got the same value back")
	}
}

func TestRefresh_RejectsReusedToken(t *testing.T) {
	authH, userH := testAuthHandler(t)
	db := testHandlerDB(t)
	username := "sprint10_refresh_reuse"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	loginRec := registerAndLogin(t, authH, userH, username, "correct-horse-battery")
	refreshCookie := cookieFrom(loginRec, authmw.RefreshTokenCookieName)

	// First use: succeeds and rotates the token.
	req1 := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req1.AddCookie(refreshCookie)
	rec1 := httptest.NewRecorder()
	authH.Refresh(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("setup: expected first refresh to succeed, got %d", rec1.Code)
	}

	// Reusing the SAME (now-rotated-away) token must fail.
	req2 := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req2.AddCookie(refreshCookie)
	rec2 := httptest.NewRecorder()
	authH.Refresh(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected reused refresh token to be rejected with %d, got %d", http.StatusUnauthorized, rec2.Code)
	}
}

func TestRefresh_NoCookie(t *testing.T) {
	authH, _ := testAuthHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	rec := httptest.NewRecorder()
	authH.Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestLogout_RevokesRefreshTokenAndClearsCookies(t *testing.T) {
	authH, userH := testAuthHandler(t)
	db := testHandlerDB(t)
	username := "sprint10_logout"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	loginRec := registerAndLogin(t, authH, userH, username, "correct-horse-battery")
	refreshCookie := cookieFrom(loginRec, authmw.RefreshTokenCookieName)

	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutReq.AddCookie(refreshCookie)
	logoutRec := httptest.NewRecorder()
	authH.Logout(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, logoutRec.Code, logoutRec.Body.String())
	}

	cleared := cookieFrom(logoutRec, authmw.AccessTokenCookieName)
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Error("expected access_token cookie to be cleared (MaxAge < 0)")
	}

	// The revoked refresh token must no longer work.
	refreshReq := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshRec := httptest.NewRecorder()
	authH.Refresh(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected a revoked refresh token to fail with %d, got %d", http.StatusUnauthorized, refreshRec.Code)
	}
}

func TestLogin_NonexistentUser_GenericError(t *testing.T) {
	authH, _ := testAuthHandler(t)

	rec := doLogin(t, authH, map[string]any{"username": "no_such_user_ever", "password": "whatever"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid username or password") {
		t.Errorf("expected the generic invalid-credentials message, got: %s", rec.Body.String())
	}
}
