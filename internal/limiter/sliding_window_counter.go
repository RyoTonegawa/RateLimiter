package limiter

import (
	"sync"
	"time"
)

type slidingCounterState struct {
	currentWindowStart time.Time
	currentCount       int
	previousCount      int
}

type SlidingWindowCounterLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	counters map[string]*slidingCounterState
}

func NewSlidingWindowCounterLimiter(limit int, window time.Duration) *SlidingWindowCounterLimiter {
	return &SlidingWindowCounterLimiter{
		limit:    limit,
		window:   window,
		counters: make(map[string]*slidingCounterState),
	}
}

func (l *SlidingWindowCounterLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	windowStart := now.Truncate(l.window)
	state, ok := l.counters[key]
	if !ok {
		state = &slidingCounterState{currentWindowStart: windowStart}
		l.counters[key] = state
	}

	if windowStart.After(state.currentWindowStart) {
		if windowStart.Sub(state.currentWindowStart) == l.window {
			state.previousCount = state.currentCount
		} else {
			state.previousCount = 0
		}
		state.currentCount = 0
		state.currentWindowStart = windowStart
	}

	elapsed := now.Sub(state.currentWindowStart)
	previousWeight := float64(l.window-elapsed) / float64(l.window)
	estimatedCount := float64(state.currentCount) + float64(state.previousCount)*previousWeight

	// System design interview points:
	// - Smoother than fixed-window counter while keeping memory O(1) per key.
	// - It is approximate: the previous window is assumed to be evenly distributed.
	// - Good practical tradeoff when exact sliding logs are too expensive at high cardinality.
	if estimatedCount >= float64(l.limit) {
		return false
	}

	state.currentCount++
	return true
}
