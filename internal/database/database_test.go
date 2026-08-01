// These tests need a real, migrated Postgres reachable at
// localhost:5432 with the noxoj/noxoj_dev_password/noxoj credentials
// (matching .env.example and docker-compose.yml's defaults). Locally:
//
//	docker compose up -d postgres
//	docker compose run --rm migrate up
//
// CI provides this via a postgres service container in ci.yml.
package database

import (
	"testing"

	"noxoj/internal/domain"
)

func testConfig() Config {
	return Config{
		Host:     "localhost",
		Port:     5432,
		User:     "noxoj",
		Password: "noxoj_dev_password",
		Name:     "noxoj",
	}
}

func TestConnect(t *testing.T) {
	db, err := Connect(testConfig())
	if err != nil {
		t.Fatalf("unexpected error connecting: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("unexpected error pinging: %v", err)
	}
}

func TestChecker(t *testing.T) {
	db, err := Connect(testConfig())
	if err != nil {
		t.Fatalf("unexpected error connecting: %v", err)
	}
	defer db.Close()

	if err := Checker(db)(); err != nil {
		t.Fatalf("expected checker to succeed, got: %v", err)
	}
}

// TestRolesSeeded proves the migration's seed data actually landed
// and that the domain.Role struct's db tags correctly map to it —
// an end-to-end check of migration -> schema -> Go query -> struct.
func TestRolesSeeded(t *testing.T) {
	db, err := Connect(testConfig())
	if err != nil {
		t.Fatalf("unexpected error connecting: %v", err)
	}
	defer db.Close()

	var roles []domain.Role
	if err := db.Select(&roles, "SELECT id, name FROM roles ORDER BY id"); err != nil {
		t.Fatalf("unexpected error querying roles: %v", err)
	}

	want := []string{"admin", "problem_setter", "contestant", "judge_operator"}
	if len(roles) != len(want) {
		t.Fatalf("expected %d seeded roles, got %d: %+v", len(want), len(roles), roles)
	}
	for i, name := range want {
		if roles[i].Name != name {
			t.Errorf("role %d: expected %q, got %q", i, name, roles[i].Name)
		}
	}
}
