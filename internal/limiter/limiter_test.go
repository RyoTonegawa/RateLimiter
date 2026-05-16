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
	t.Cleanup(limiter.Close)

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

func TestTokenBucketAllowsInstantBurstUpToCapacity(t *testing.T) {
	limiter := NewTokenBucketLimiter(5, 1)
	now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	allowed := 0
	for range 5 {
		if limiter.Allow("client") {
			allowed++
		}
	}

	if allowed != 5 {
		t.Fatalf("token bucket should allow an instant burst up to capacity: got %d", allowed)
	}
	if limiter.Allow("client") {
		t.Fatal("request over burst capacity should be rejected")
	}
}

func TestLeakingBucketCanFillQueueAndRejectUntilLeakCatchesUp(t *testing.T) {
	now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
	limiter := NewLeakingBucketLimiterWithClock(3, time.Second, func() time.Time { return now })

	for range 3 {
		if !limiter.Allow("client") {
			t.Fatal("request should be admitted while queue has capacity")
		}
	}
	if limiter.Allow("client") {
		t.Fatal("full leaking bucket should reject new arrivals")
	}
	if got := len(limiter.buckets["client"].queue); got != 3 {
		t.Fatalf("queue should contain admitted requests: got %d", got)
	}

	now = now.Add(time.Second)
	limiter.drainState(limiter.buckets["client"], now)
	if !limiter.Allow("client") {
		t.Fatal("one request should be admitted after one leak interval")
	}
	if got := len(limiter.buckets["client"].queue); got != 3 {
		t.Fatalf("one old request should drain before a new one is enqueued: got %d", got)
	}
}

func TestLeakingBucketWorkerDrainsQueueAtFixedRate(t *testing.T) {
	limiter := NewLeakingBucketLimiter(1, 20*time.Millisecond)
	t.Cleanup(limiter.Close)

	if !limiter.Allow("client") {
		t.Fatal("first request should be allowed")
	}
	if limiter.Allow("client") {
		t.Fatal("full queue should reject before worker drains")
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if limiter.Allow("client") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("worker should drain one queued request at a fixed interval")
}

func TestFixedWindowCounterAllowsBoundaryBurst(t *testing.T) {
	limiter := NewFixedWindowCounterLimiter(5, time.Minute)
	now := time.Date(2026, 5, 11, 9, 0, 59, 900*int(time.Millisecond), time.UTC)
	limiter.now = func() time.Time { return now }

	firstWindowAllowed := 0
	for range 5 {
		if limiter.Allow("client") {
			firstWindowAllowed++
		}
	}

	now = time.Date(2026, 5, 11, 9, 1, 0, 100*int(time.Millisecond), time.UTC)
	secondWindowAllowed := 0
	for range 5 {
		if limiter.Allow("client") {
			secondWindowAllowed++
		}
	}

	if firstWindowAllowed != 5 || secondWindowAllowed != 5 {
		t.Fatalf("fixed window should allow limit requests on both sides of a boundary: got %d and %d", firstWindowAllowed, secondWindowAllowed)
	}
}

func TestSlidingWindowLogStoresOneTimestampPerAcceptedRequest(t *testing.T) {
	limiter := NewSlidingWindowLogLimiter(100, time.Minute)
	now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	for i := range 100 {
		now = now.Add(time.Millisecond)
		if !limiter.Allow("client") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if got := len(limiter.requests["client"]); got != 100 {
		t.Fatalf("sliding log stores every accepted timestamp in the active window: got %d", got)
	}
}

func TestSlidingWindowCounterApproximationCanRejectEvenWhenExactLogWouldAllow(t *testing.T) {
	counter := NewSlidingWindowCounterLimiter(5, time.Minute)
	log := NewSlidingWindowLogLimiter(5, time.Minute)
	now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
	counter.now = func() time.Time { return now }
	log.now = func() time.Time { return now }

	for range 5 {
		now = time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
		if !counter.Allow("client") {
			t.Fatal("counter should allow initial previous-window request")
		}
		if !log.Allow("client") {
			t.Fatal("log should allow initial previous-window request")
		}
	}

	now = time.Date(2026, 5, 11, 9, 1, 1, 0, time.UTC)

	if !log.Allow("client") {
		t.Fatal("exact sliding log should allow because the old requests are outside the last minute")
	}
	if !counter.Allow("client") {
		t.Fatal("counter should allow the first request in the new minute")
	}
	if !log.Allow("client") {
		t.Fatal("exact sliding log should allow another request because the old requests are outside the last minute")
	}
	if counter.Allow("client") {
		t.Fatal("sliding window counter can falsely reject because it weights the entire previous fixed window")
	}
}
