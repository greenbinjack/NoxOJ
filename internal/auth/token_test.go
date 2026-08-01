package auth

import (
	"reflect"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestGenerateAndParseAccessToken(t *testing.T) {
	secret := []byte("test-secret")
	userID := uuid.New()
	roles := []string{"contestant"}

	token, err := GenerateAccessToken(userID, roles, secret)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	got, err := ParseAccessToken(token, secret)
	if err != nil {
		t.Fatalf("unexpected error parsing token: %v", err)
	}
	if got.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, got.UserID)
	}
	if !reflect.DeepEqual(got.Roles, roles) {
		t.Errorf("expected roles %v, got %v", roles, got.Roles)
	}
}

func TestParseAccessToken_WrongSecret(t *testing.T) {
	token, err := GenerateAccessToken(uuid.New(), nil, []byte("secret-a"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := ParseAccessToken(token, []byte("secret-b")); err == nil {
		t.Error("expected an error parsing a token signed with a different secret, got nil")
	}
}

func TestParseAccessToken_Expired(t *testing.T) {
	secret := []byte("test-secret")
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := ParseAccessToken(token, secret); err == nil {
		t.Error("expected an error parsing an expired token, got nil")
	}
}

func TestParseAccessToken_RejectsNoneAlgorithm(t *testing.T) {
	// A forged token claiming alg=none — must never be accepted,
	// regardless of the secret used to "verify" it.
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, c).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("unexpected error forging token: %v", err)
	}

	if _, err := ParseAccessToken(token, []byte("test-secret")); err == nil {
		t.Error("expected alg=none token to be rejected, got nil error")
	}
}
