package limiter

import (
	"sync"
	"time"
)

type tokenBucketState struct {
	tokens     float64
	lastRefill time.Time
}

type TokenBucketLimiter struct {
	mu         sync.Mutex
	capacity   float64
	refillRate float64
	buckets    map[string]*tokenBucketState
	now        func() time.Time
}

func NewTokenBucketLimiter(capacity int, refillPerSecond float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		capacity:   float64(capacity),
		refillRate: refillPerSecond,
		buckets:    make(map[string]*tokenBucketState),
		now:        time.Now,
	}
}

func (l *TokenBucketLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	state, ok := l.buckets[key]
	if !ok {
		state = &tokenBucketState{
			tokens:     l.capacity,
			lastRefill: now,
		}
		l.buckets[key] = state
	}

	elapsed := now.Sub(state.lastRefill).Seconds()
	state.tokens = min(l.capacity, state.tokens+elapsed*l.refillRate)
	state.lastRefill = now

	// System design interview points:
	// - Good when clients need occasional bursts while preserving a long-term average rate.
	// - Capacity controls maximum burst size; refill rate controls sustained throughput.
	// - Distributed implementations usually store token count + last-refill timestamp in Redis and update atomically with Lua.
	if state.tokens < 1 {
		return false
	}

	state.tokens--
	return true
}
