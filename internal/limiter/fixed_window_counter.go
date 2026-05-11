package limiter

import (
	"sync"
	"time"
)

type fixedWindowState struct {
	windowStart time.Time
	count       int
}

type FixedWindowCounterLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	counters map[string]*fixedWindowState
	now      func() time.Time
}

func NewFixedWindowCounterLimiter(limit int, window time.Duration) *FixedWindowCounterLimiter {
	return &FixedWindowCounterLimiter{
		limit:    limit,
		window:   window,
		counters: make(map[string]*fixedWindowState),
		now:      time.Now,
	}
}

func (l *FixedWindowCounterLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	state, ok := l.counters[key]
	// 存在しないか、次のWindowに移っている場合は新しいカウンタを作成する
	if !ok || now.Sub(state.windowStart) >= l.window {
		state = &fixedWindowState{
			windowStart: now.Truncate(l.window),
			count:       0,
		}
		l.counters[key] = state
	}

	// System design interview points:
	// - Very cheap: one counter per key per active window.
	// - Main weakness is boundary burst: a client can send limit requests at the end of one window and another limit at the start of the next.
	// - Distributed implementations commonly use Redis INCR with TTL, but clock/window alignment matters across nodes.
	// 今のWindowカウンタがlimitよりも大きい場合はfalse
	if state.count >= l.limit {
		return false
	}
	// カウンタ内に収まる場合はインクリメントして返す
	state.count++
	return true
}
