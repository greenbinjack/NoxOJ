package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"noxoj/internal/auth"
	"noxoj/internal/database"
	"noxoj/internal/middleware"
	"noxoj/internal/repository"
)

func testUserHandler(t *testing.T) (*UserHandler, *sqlx.DB) {
	t.Helper()
	db := testHandlerDB(t)
	return NewUserHandler(zerolog.Nop(), repository.NewUserRepository(db)), db
}

func testHandlerDB(t *testing.T) *sqlx.DB {
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
	return db
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

func doRegister(t *testing.T, h *UserHandler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, h.Register, "/register", body)
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

func TestMe_ReturnsFreshProfileAndTokenRoles(t *testing.T) {
	h, db := testUserHandler(t)
	username := "sprint12_me_ok"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	reg := doRegister(t, h, map[string]any{"username": username, "password": "correct-horse-battery", "display_name": "Me Test"})
	if reg.Code != http.StatusCreated {
		t.Fatalf("setup: expected registration to succeed, got %d: %s", reg.Code, reg.Body.String())
	}
	var registered registerResponse
	if err := json.Unmarshal(reg.Body.Bytes(), &registered); err != nil {
		t.Fatalf("unexpected error decoding register response: %v", err)
	}

	// Deliberately embed a DIFFERENT role in the token than what's
	// actually in the database — proves Roles in the response comes
	// from the token (context), not a fresh DB query, per Sprint 11's
	// design (a promotion/demotion only takes effect on next
	// login/refresh, and /me should reflect exactly that, not bypass it).
	token, err := auth.GenerateAccessToken(uuid.MustParse(registered.ID), []string{"admin"}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	protected := middleware.Authenticate(testJWTSecret)(http.HandlerFunc(h.Me))
	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	req.AddCookie(&http.Cookie{Name: middleware.AccessTokenCookieName, Value: token})
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp meResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected error decoding response: %v", err)
	}
	if resp.Username != username || resp.DisplayName != "Me Test" || resp.Rating != 1500 {
		t.Errorf("unexpected profile fields: %+v", resp)
	}
	if len(resp.Roles) != 1 || resp.Roles[0] != "admin" {
		t.Errorf("expected roles to come from the token (%v), got %v", []string{"admin"}, resp.Roles)
	}
}

func TestMe_RequiresAuthentication(t *testing.T) {
	h, _ := testUserHandler(t)

	protected := middleware.Authenticate(testJWTSecret)(http.HandlerFunc(h.Me))
	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}
