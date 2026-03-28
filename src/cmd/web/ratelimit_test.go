package main

import (
	"testing"
	"time"
)

func TestLoginRateLimiterAllowsInitialAttempts(t *testing.T) {
	t.Parallel()
	rl := newLoginRateLimiter(5, time.Minute)
	defer rl.Stop()
	for i := 0; i < 5; i++ {
		if !rl.Allow("alice") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
}

func TestLoginRateLimiterBlocksAfterLimit(t *testing.T) {
	t.Parallel()
	rl := newLoginRateLimiter(3, time.Minute)
	defer rl.Stop()
	for i := 0; i < 3; i++ {
		rl.Allow("bob")
	}
	if rl.Allow("bob") {
		t.Fatal("4th attempt should be blocked")
	}
}

func TestLoginRateLimiterIsolatesUsers(t *testing.T) {
	t.Parallel()
	rl := newLoginRateLimiter(2, time.Minute)
	defer rl.Stop()
	rl.Allow("carol")
	rl.Allow("carol")
	// carol is at limit, but dave should still be allowed.
	if !rl.Allow("dave") {
		t.Fatal("dave should not be affected by carol's rate limit")
	}
}

func TestLoginRateLimiterResetsAfterWindow(t *testing.T) {
	t.Parallel()
	rl := newLoginRateLimiter(2, 50*time.Millisecond)
	defer rl.Stop()
	rl.Allow("eve")
	rl.Allow("eve")
	if rl.Allow("eve") {
		t.Fatal("should be blocked at limit")
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.Allow("eve") {
		t.Fatal("should be allowed after window expires")
	}
}

func TestLoginRateLimiterResetOnSuccess(t *testing.T) {
	t.Parallel()
	rl := newLoginRateLimiter(3, time.Minute)
	defer rl.Stop()
	rl.Allow("frank")
	rl.Allow("frank")
	rl.Reset("frank")
	// After reset, counter should be back to 0.
	for i := 0; i < 3; i++ {
		if !rl.Allow("frank") {
			t.Fatalf("attempt %d after reset should be allowed", i+1)
		}
	}
}
