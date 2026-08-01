package ratelimit

import (
	"testing"
	"time"
)

func TestLoginLimiter_AllowsUntilThreshold(t *testing.T) {
	l := NewLoginLimiter(3, time.Hour)
	username := "someuser"

	for i := 0; i < 2; i++ {
		if !l.Allowed(username) {
			t.Fatalf("expected attempt %d to be allowed", i+1)
		}
		l.RecordFailure(username)
	}

	if !l.Allowed(username) {
		t.Fatal("expected 3rd attempt to still be allowed (threshold not yet reached)")
	}
	l.RecordFailure(username)

	if l.Allowed(username) {
		t.Fatal("expected username to be locked out after reaching the failure threshold")
	}
}

func TestLoginLimiter_UnlocksAfterDuration(t *testing.T) {
	l := NewLoginLimiter(1, 10*time.Millisecond)
	username := "someuser"

	l.RecordFailure(username)
	if l.Allowed(username) {
		t.Fatal("expected lockout immediately after reaching the threshold")
	}

	time.Sleep(20 * time.Millisecond)
	if !l.Allowed(username) {
		t.Fatal("expected lockout to expire after lockoutDuration")
	}
}

func TestLoginLimiter_SuccessClearsFailures(t *testing.T) {
	l := NewLoginLimiter(2, time.Hour)
	username := "someuser"

	l.RecordFailure(username)
	l.RecordSuccess(username)
	l.RecordFailure(username)

	if !l.Allowed(username) {
		t.Fatal("expected only 1 failure to be tracked after a success reset the counter")
	}
}

func TestLoginLimiter_UsernamesAreIndependent(t *testing.T) {
	l := NewLoginLimiter(1, time.Hour)

	l.RecordFailure("alice")

	if !l.Allowed("bob") {
		t.Fatal("expected a different username to be unaffected by alice's lockout")
	}
}
