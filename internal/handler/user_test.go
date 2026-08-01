package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"noxoj/internal/config"
	"noxoj/internal/database"
	"noxoj/internal/ratelimit"
	"noxoj/internal/repository"
)

var testJWTSecret = []byte("test-secret-for-handler-tests")

func testUserHandler(t *testing.T) (*UserHandler, *sqlx.DB) {
	t.Helper()
	db, err := database.Connect(database.Config{
		Host:     "localhost",
		Port:     5432,
		User:     "noxoj",
		Password: "noxoj_dev_password",
		Name:     "noxoj",
	})
	if err != nil {
		t.Fatalf("unexpected error connecting: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	limiter := ratelimit.NewLoginLimiter(5, 15*time.Minute)
	h := NewUserHandler(zerolog.Nop(), repository.NewUserRepository(db), testJWTSecret, limiter, config.Development)
	return h, db
}

func doRegister(t *testing.T, h *UserHandler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, h.Register, "/register", body)
}

func doLogin(t *testing.T, h *UserHandler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, h.Login, "/login", body)
}

func doRequest(t *testing.T, fn http.HandlerFunc, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("unexpected error marshaling request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

func TestRegister_Success(t *testing.T) {
	h, db := testUserHandler(t)
	username := "sprint8_handler_ok"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	rec := doRegister(t, h, map[string]any{
		"username":     username,
		"password":     "correct-horse-battery",
		"display_name": "Handler Test",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp registerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected error decoding response: %v", err)
	}
	if resp.Username != username || resp.Rating != 1500 || resp.ID == "" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "password") {
		t.Error("response body must never mention the password or its hash")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	h, db := testUserHandler(t)
	username := "sprint8_handler_dup"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	first := doRegister(t, h, map[string]any{"username": username, "password": "correct-horse-battery", "display_name": "First"})
	if first.Code != http.StatusCreated {
		t.Fatalf("expected first registration to succeed, got %d: %s", first.Code, first.Body.String())
	}

	second := doRegister(t, h, map[string]any{"username": username, "password": "another-password", "display_name": "Second"})
	if second.Code != http.StatusConflict {
		t.Fatalf("expected %d, got %d: %s", http.StatusConflict, second.Code, second.Body.String())
	}
}

func TestRegister_InvalidInput(t *testing.T) {
	h, _ := testUserHandler(t)

	cases := map[string]map[string]any{
		"username too short":   {"username": "ab", "password": "correct-horse-battery", "display_name": "X"},
		"password too short":   {"username": "sprint8_iv_1", "password": "short", "display_name": "X"},
		"missing display name": {"username": "sprint8_iv_2", "password": "correct-horse-battery", "display_name": ""},
		"invalid email":        {"username": "sprint8_iv_3", "password": "correct-horse-battery", "display_name": "X", "email": "not-an-email"},
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := doRegister(t, h, body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestLogin_Success(t *testing.T) {
	h, db := testUserHandler(t)
	username := "sprint9_login_ok"
	password := "correct-horse-battery"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	reg := doRegister(t, h, map[string]any{"username": username, "password": password, "display_name": "Login Test"})
	if reg.Code != http.StatusCreated {
		t.Fatalf("setup: expected registration to succeed, got %d: %s", reg.Code, reg.Body.String())
	}

	rec := doLogin(t, h, map[string]any{"username": username, "password": password})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "access_token" {
			found = true
			if !c.HttpOnly {
				t.Error("expected access_token cookie to be HttpOnly")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Error("expected access_token cookie to be SameSite=Strict")
			}
			if c.Secure {
				t.Error("expected Secure=false in development (no TLS on localhost)")
			}
			if c.Value == "" {
				t.Error("expected a non-empty token value")
			}
		}
	}
	if !found {
		t.Fatal("expected an access_token cookie to be set")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	h, db := testUserHandler(t)
	username := "sprint9_login_wrongpw"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	doRegister(t, h, map[string]any{"username": username, "password": "correct-horse-battery", "display_name": "X"})

	rec := doLogin(t, h, map[string]any{"username": username, "password": "totally-wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
}

func TestLogin_NonexistentUser(t *testing.T) {
	h, _ := testUserHandler(t)

	rec := doLogin(t, h, map[string]any{"username": "no_such_user_ever", "password": "whatever-password"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
	// Must be the same generic message as a wrong password — never
	// reveal whether the username itself was the problem.
	if !strings.Contains(rec.Body.String(), "invalid username or password") {
		t.Errorf("expected the generic invalid-credentials message, got: %s", rec.Body.String())
	}
}

func TestLogin_LockedOutAfterRepeatedFailures(t *testing.T) {
	h, db := testUserHandler(t)
	username := "sprint9_login_lockout"
	password := "correct-horse-battery"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	doRegister(t, h, map[string]any{"username": username, "password": password, "display_name": "X"})

	for i := 0; i < 5; i++ {
		rec := doLogin(t, h, map[string]any{"username": username, "password": "wrong"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected %d, got %d", i+1, http.StatusUnauthorized, rec.Code)
		}
	}

	// 6th attempt, even with the CORRECT password, should now be locked out.
	rec := doLogin(t, h, map[string]any{"username": username, "password": password})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected %d after repeated failures, got %d: %s", http.StatusTooManyRequests, rec.Code, rec.Body.String())
	}
}
