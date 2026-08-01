package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"noxoj/internal/auth"
	"noxoj/internal/database"
	"noxoj/internal/middleware"
	"noxoj/internal/repository"
	"noxoj/internal/tokenstore"
)

func testUserHandler(t *testing.T) (*UserHandler, *sqlx.DB) {
	t.Helper()
	db := testHandlerDB(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	t.Cleanup(func() { redisClient.Close() })
	refreshTokens := tokenstore.NewRefreshTokenStore(redisClient)
	return NewUserHandler(zerolog.Nop(), repository.NewUserRepository(db), refreshTokens), db
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

// doDeactivate simulates the route Deactivate actually runs behind:
// Authenticate (for the actor's ID in context) wrapping a chi route
// with an {id} URL parameter, exactly like main.go wires it —
// calling h.Deactivate directly wouldn't have either.
func doDeactivate(t *testing.T, h *UserHandler, actorToken string, targetID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", targetID.String())

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetID.String()+"/deactivate", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req.AddCookie(&http.Cookie{Name: middleware.AccessTokenCookieName, Value: actorToken})

	rec := httptest.NewRecorder()
	middleware.Authenticate(testJWTSecret)(http.HandlerFunc(h.Deactivate)).ServeHTTP(rec, req)
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

func TestDeactivate_Success(t *testing.T) {
	h, db := testUserHandler(t)
	adminUsername := "sprint14_deactivate_admin"
	targetUsername := "sprint14_deactivate_target"
	t.Cleanup(func() {
		db.MustExec("DELETE FROM audit_log WHERE actor_id IN (SELECT id FROM users WHERE username = $1)", adminUsername)
		db.MustExec("DELETE FROM users WHERE username IN ($1, $2)", adminUsername, targetUsername)
	})

	adminReg := doRegister(t, h, map[string]any{"username": adminUsername, "password": "correct-horse-battery", "display_name": "Admin"})
	targetReg := doRegister(t, h, map[string]any{"username": targetUsername, "password": "correct-horse-battery", "display_name": "Target"})
	var admin, target registerResponse
	json.Unmarshal(adminReg.Body.Bytes(), &admin)
	json.Unmarshal(targetReg.Body.Bytes(), &target)

	adminToken, err := auth.GenerateAccessToken(uuid.MustParse(admin.ID), []string{"admin"}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	rec := doDeactivate(t, h, adminToken, uuid.MustParse(target.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var deletedAt *string
	if err := db.Get(&deletedAt, "SELECT deleted_at FROM users WHERE id = $1", target.ID); err != nil {
		t.Fatalf("unexpected error checking deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Error("expected deleted_at to be set after deactivation")
	}
}

func TestDeactivate_PreventsSelfDeactivation(t *testing.T) {
	h, db := testUserHandler(t)
	username := "sprint14_deactivate_self"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	reg := doRegister(t, h, map[string]any{"username": username, "password": "correct-horse-battery", "display_name": "Self"})
	var registered registerResponse
	json.Unmarshal(reg.Body.Bytes(), &registered)

	token, err := auth.GenerateAccessToken(uuid.MustParse(registered.ID), []string{"admin"}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	rec := doDeactivate(t, h, token, uuid.MustParse(registered.ID))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}

	var deletedAt *string
	if err := db.Get(&deletedAt, "SELECT deleted_at FROM users WHERE id = $1", registered.ID); err != nil {
		t.Fatalf("unexpected error checking deleted_at: %v", err)
	}
	if deletedAt != nil {
		t.Error("expected the blocked self-deactivation to leave deleted_at unset")
	}
}

func TestDeactivate_NotFound(t *testing.T) {
	h, db := testUserHandler(t)
	adminUsername := "sprint14_deact_notfound_admin"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", adminUsername) })

	adminReg := doRegister(t, h, map[string]any{"username": adminUsername, "password": "correct-horse-battery", "display_name": "Admin"})
	var admin registerResponse
	json.Unmarshal(adminReg.Body.Bytes(), &admin)

	adminToken, err := auth.GenerateAccessToken(uuid.MustParse(admin.ID), []string{"admin"}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	rec := doDeactivate(t, h, adminToken, uuid.New())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestDeactivate_RevokesTargetSessions(t *testing.T) {
	h, db := testUserHandler(t)
	adminUsername := "sprint14_deact_revoke_admin"
	targetUsername := "sprint14_deact_revoke_target"
	t.Cleanup(func() {
		db.MustExec("DELETE FROM audit_log WHERE actor_id IN (SELECT id FROM users WHERE username = $1)", adminUsername)
		db.MustExec("DELETE FROM users WHERE username IN ($1, $2)", adminUsername, targetUsername)
	})

	adminReg := doRegister(t, h, map[string]any{"username": adminUsername, "password": "correct-horse-battery", "display_name": "Admin"})
	targetReg := doRegister(t, h, map[string]any{"username": targetUsername, "password": "correct-horse-battery", "display_name": "Target"})
	var admin, target registerResponse
	json.Unmarshal(adminReg.Body.Bytes(), &admin)
	json.Unmarshal(targetReg.Body.Bytes(), &target)

	adminToken, err := auth.GenerateAccessToken(uuid.MustParse(admin.ID), []string{"admin"}, testJWTSecret)
	if err != nil {
		t.Fatalf("unexpected error generating admin token: %v", err)
	}

	targetID := uuid.MustParse(target.ID)
	refreshToken, err := h.refreshTokens.Issue(context.Background(), targetID)
	if err != nil {
		t.Fatalf("setup: unexpected error issuing refresh token: %v", err)
	}

	rec := doDeactivate(t, h, adminToken, targetID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	if _, err := h.refreshTokens.Consume(context.Background(), refreshToken); err == nil {
		t.Error("expected the deactivated user's refresh token to be revoked")
	}
}
