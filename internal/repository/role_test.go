package repository

import (
	"context"
	"errors"
	"testing"

	"noxoj/internal/domain"
)

func TestRoleRepository_AssignAndGetRoleNames(t *testing.T) {
	db := testDB(t)
	users := NewUserRepository(db)
	roles := NewRoleRepository(db)
	ctx := context.Background()

	username := "sprint11_role_assign"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	user, err := users.Create(ctx, &domain.User{Username: username, DisplayName: "Role Test"})
	if err != nil {
		t.Fatalf("unexpected error creating user: %v", err)
	}

	if err := roles.AssignRole(ctx, user.ID, domain.RoleJudgeOperator); err != nil {
		t.Fatalf("unexpected error assigning role: %v", err)
	}

	names, err := roles.GetRoleNames(ctx, user.ID)
	if err != nil {
		t.Fatalf("unexpected error getting role names: %v", err)
	}
	if len(names) != 1 || names[0] != domain.RoleJudgeOperator {
		t.Errorf("expected [%q], got %v", domain.RoleJudgeOperator, names)
	}
}

func TestRoleRepository_AssignRole_Idempotent(t *testing.T) {
	db := testDB(t)
	users := NewUserRepository(db)
	roles := NewRoleRepository(db)
	ctx := context.Background()

	username := "sprint11_role_idempotent"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	user, err := users.Create(ctx, &domain.User{Username: username, DisplayName: "Idempotent Test"})
	if err != nil {
		t.Fatalf("unexpected error creating user: %v", err)
	}

	if err := roles.AssignRole(ctx, user.ID, domain.RoleAdmin); err != nil {
		t.Fatalf("unexpected error on first assign: %v", err)
	}
	if err := roles.AssignRole(ctx, user.ID, domain.RoleAdmin); err != nil {
		t.Fatalf("expected re-assigning the same role to succeed silently, got: %v", err)
	}

	names, err := roles.GetRoleNames(ctx, user.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("expected exactly 1 role after assigning the same one twice, got %v", names)
	}
}

func TestRoleRepository_AssignRole_UnknownRole(t *testing.T) {
	db := testDB(t)
	users := NewUserRepository(db)
	roles := NewRoleRepository(db)
	ctx := context.Background()

	username := "sprint11_role_unknown"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	user, err := users.Create(ctx, &domain.User{Username: username, DisplayName: "Unknown Role Test"})
	if err != nil {
		t.Fatalf("unexpected error creating user: %v", err)
	}

	err = roles.AssignRole(ctx, user.ID, "definitely_not_a_real_role")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound, got %v", err)
	}
}

func TestRoleRepository_GetRoleNames_NoRoles(t *testing.T) {
	db := testDB(t)
	users := NewUserRepository(db)
	roles := NewRoleRepository(db)
	ctx := context.Background()

	username := "sprint11_role_none"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	user, err := users.Create(ctx, &domain.User{Username: username, DisplayName: "No Roles Test"})
	if err != nil {
		t.Fatalf("unexpected error creating user: %v", err)
	}

	names, err := roles.GetRoleNames(ctx, user.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected no roles for a freshly-created user via plain Create, got %v", names)
	}
}

func TestUserRepository_CreateWithRole(t *testing.T) {
	db := testDB(t)
	users := NewUserRepository(db)
	roles := NewRoleRepository(db)
	ctx := context.Background()

	username := "sprint11_create_with_role"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	created, err := users.CreateWithRole(ctx, &domain.User{Username: username, DisplayName: "X"}, domain.DefaultRole)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, err := roles.GetRoleNames(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 1 || names[0] != domain.DefaultRole {
		t.Errorf("expected [%q], got %v", domain.DefaultRole, names)
	}
}

func TestUserRepository_CreateWithRole_RollsBackOnUnknownRole(t *testing.T) {
	db := testDB(t)
	users := NewUserRepository(db)
	ctx := context.Background()

	username := "sprint11_create_bad_role"
	// No t.Cleanup delete needed if the transaction correctly rolls
	// back — this test itself is the proof either way, but clean up
	// defensively in case it doesn't.
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	_, err := users.CreateWithRole(ctx, &domain.User{Username: username, DisplayName: "X"}, "not_a_real_role")
	if err == nil {
		t.Fatal("expected an error assigning a nonexistent default role")
	}

	var count int
	if err := db.Get(&count, "SELECT count(*) FROM users WHERE username = $1", username); err != nil {
		t.Fatalf("unexpected error checking rollback: %v", err)
	}
	if count != 0 {
		t.Errorf("expected the user insert to roll back when role assignment failed, but found %d row(s)", count)
	}
}
