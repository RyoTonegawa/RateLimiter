package limiter

import "time"

func NewTokenBucketLimiterWithClock(capacity int, refillPerSecond float64, now func() time.Time) *TokenBucketLimiter {
	limiter := NewTokenBucketLimiter(capacity, refillPerSecond)
	limiter.now = now
	return limiter
}

func NewLeakingBucketLimiterWithClock(capacity int, leakInterval time.Duration, now func() time.Time) *LeakingBucketLimiter {
	limiter := NewLeakingBucketLimiter(capacity, leakInterval)
	limiter.now = now
	return limiter
}

func NewFixedWindowCounterLimiterWithClock(limit int, window time.Duration, now func() time.Time) *FixedWindowCounterLimiter {
	limiter := NewFixedWindowCounterLimiter(limit, window)
	limiter.now = now
	return limiter
}

func NewSlidingWindowLogLimiterWithClock(limit int, window time.Duration, now func() time.Time) *SlidingWindowLogLimiter {
	limiter := NewSlidingWindowLogLimiter(limit, window)
	limiter.now = now
	return limiter
}

func NewSlidingWindowCounterLimiterWithClock(limit int, window time.Duration, now func() time.Time) *SlidingWindowCounterLimiter {
	limiter := NewSlidingWindowCounterLimiter(limit, window)
	limiter.now = now
	return limiter
}
