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
	now      func() time.Time
}

func NewSlidingWindowCounterLimiter(limit int, window time.Duration) *SlidingWindowCounterLimiter {
	return &SlidingWindowCounterLimiter{
		limit:    limit,
		window:   window,
		counters: make(map[string]*slidingCounterState),
		now:      time.Now,
	}
}

func (l *SlidingWindowCounterLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	// 現在自国が属しているWindowの開始時刻を取得
	// 時刻をWindowの単位で切り捨て取得
	windowStart := now.Truncate(l.window)
	state, ok := l.counters[key]
	if !ok {
		state = &slidingCounterState{currentWindowStart: windowStart}
		l.counters[key] = state
	}
	//currentとして見ているWindowの後の場合
	if windowStart.After(state.currentWindowStart) {
		//どのくらい進んだかを判断する。
		// ちょうどwindowだけか、windowよりも長く進んでいるか
		if windowStart.Sub(state.currentWindowStart) == l.window {
			// ちょうどwindowだけ進んだ場合、currentだったものをpreviousとおく
			state.previousCount = state.currentCount
		} else {
			// もはや前のwindowのカウントは関係ないのでゼロにリセット
			state.previousCount = 0
		}
		state.currentCount = 0
		state.currentWindowStart = windowStart
	}
	// ここから経過時間に合わせてwindowに重み付けしてpreviousのうちのこすカウントを算出する
	elapsed := now.Sub(state.currentWindowStart)
	// 前のwindowがどのくらい残っているか？をカウントに掛け、その分をcurrentと足してCountとする
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
