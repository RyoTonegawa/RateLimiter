package limiter

import (
	"testing"
	"time"
)

func TestTokenBucketLimiterRejectsWhenBucketIsEmpty(t *testing.T) {
	limiter := NewTokenBucketLimiter(2, 0)

	if !limiter.Allow("client") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("client") {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow("client") {
		t.Fatal("third request should be rejected")
	}
}

func TestLeakingBucketLimiterRejectsWhenBucketIsFull(t *testing.T) {
	limiter := NewLeakingBucketLimiter(2, time.Hour)

	if !limiter.Allow("client") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("client") {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow("client") {
		t.Fatal("third request should be rejected")
	}
}

func TestFixedWindowCounterLimiterRejectsOverLimit(t *testing.T) {
	limiter := NewFixedWindowCounterLimiter(2, time.Minute)

	if !limiter.Allow("client") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("client") {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow("client") {
		t.Fatal("third request should be rejected")
	}
}

func TestSlidingWindowLogLimiterRejectsOverLimit(t *testing.T) {
	limiter := NewSlidingWindowLogLimiter(2, time.Minute)

	if !limiter.Allow("client") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("client") {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow("client") {
		t.Fatal("third request should be rejected")
	}
}

func TestSlidingWindowCounterLimiterRejectsOverLimit(t *testing.T) {
	limiter := NewSlidingWindowCounterLimiter(2, time.Minute)

	if !limiter.Allow("client") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("client") {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow("client") {
		t.Fatal("third request should be rejected")
	}
}
