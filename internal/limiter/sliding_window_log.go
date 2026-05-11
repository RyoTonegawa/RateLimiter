package limiter

import (
	"sync"
	"time"
)

type SlidingWindowLogLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	requests map[string][]time.Time
	now      func() time.Time
}

func NewSlidingWindowLogLimiter(limit int, window time.Duration) *SlidingWindowLogLimiter {
	return &SlidingWindowLogLimiter{
		limit:    limit,
		window:   window,
		requests: make(map[string][]time.Time),
		now:      time.Now,
	}
}

func (l *SlidingWindowLogLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)
	timestamps := l.requests[key]

	firstActive := 0
	for firstActive < len(timestamps) && !timestamps[firstActive].After(cutoff) {
		firstActive++
	}
	timestamps = timestamps[firstActive:]

	// System design interview points:
	// - Most accurate of the common rolling-window approaches because it stores exact request times.
	// - Memory usage grows with traffic: O(number of accepted requests inside the window).
	// - Distributed versions often use Redis sorted sets, trimming old timestamps and counting current ones atomically.
	if len(timestamps) >= l.limit {
		l.requests[key] = timestamps
		return false
	}

	timestamps = append(timestamps, now)
	l.requests[key] = timestamps
	return true
}
