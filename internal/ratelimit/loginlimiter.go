// Package ratelimit provides simple, in-process rate limiting.
package ratelimit

import (
	"sync"
	"time"
)

type attempt struct {
	failures    int
	lockedUntil time.Time
}

// LoginLimiter tracks failed login attempts per username, in memory.
// This is single-instance only — state isn't shared across processes.
// That's an honest limitation, not an oversight: NoxOJ is a single
// monolith right now (no autoscaling until Phase 3), so there's only
// ever one instance to track against. Once that changes, this needs
// a shared store (Redis, which Sprint 10 introduces for a different
// reason — a natural place to revisit this, not something to solve
// ahead of actually needing it).
type LoginLimiter struct {
	mu               sync.Mutex
	attempts         map[string]*attempt
	maxFailures      int
	lockoutDuration  time.Duration
}

// NewLoginLimiter creates a limiter that locks out a username after
// maxFailures consecutive failed attempts, for lockoutDuration.
func NewLoginLimiter(maxFailures int, lockoutDuration time.Duration) *LoginLimiter {
	return &LoginLimiter{
		attempts:        make(map[string]*attempt),
		maxFailures:     maxFailures,
		lockoutDuration: lockoutDuration,
	}
}

// Allowed reports whether a login attempt for username should be
// permitted right now.
func (l *LoginLimiter) Allowed(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[username]
	if !ok || a.failures < l.maxFailures {
		return true
	}
	return time.Now().After(a.lockedUntil)
}

// RecordFailure registers a failed login attempt for username.
func (l *LoginLimiter) RecordFailure(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[username]
	if !ok {
		a = &attempt{}
		l.attempts[username] = a
	}
	a.failures++
	if a.failures >= l.maxFailures {
		a.lockedUntil = time.Now().Add(l.lockoutDuration)
	}
}

// RecordSuccess clears any tracked failures for username.
func (l *LoginLimiter) RecordSuccess(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, username)
}
