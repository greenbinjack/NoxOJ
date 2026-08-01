package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("unexpected error hashing: %v", err)
	}

	if err := CheckPassword(hash, "correct-horse-battery-staple"); err != nil {
		t.Errorf("expected correct password to verify, got error: %v", err)
	}

	if err := CheckPassword(hash, "wrong-password"); err == nil {
		t.Error("expected wrong password to fail verification, got nil error")
	}
}

func TestHashPassword_SameInputDifferentOutput(t *testing.T) {
	hash1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("expected two hashes of the same password to differ (salted), got identical hashes")
	}
}
