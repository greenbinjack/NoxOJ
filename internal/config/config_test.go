package config

import "testing"

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Environment != Development {
		t.Errorf("expected default environment %q, got %q", Development, cfg.Environment)
	}
	if cfg.Port != 8081 {
		t.Errorf("expected default port 8081, got %d", cfg.Port)
	}
}

func TestLoad_EnvVarsOverrideDefaults(t *testing.T) {
	t.Setenv("PORT", "9000")
	t.Setenv("ENVIRONMENT", "production")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Environment != Production {
		t.Errorf("expected environment %q, got %q", Production, cfg.Environment)
	}
	if cfg.Port != 9000 {
		t.Errorf("expected port 9000, got %d", cfg.Port)
	}
}

func TestLoad_RejectsInvalidEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "staging-typo")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an invalid ENVIRONMENT value, got nil")
	}
}
