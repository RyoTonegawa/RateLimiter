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
	// 現在時刻からwindow分前の時点の時刻を取得
	cutoff := now.Add(-l.window)
	timestamps := l.requests[key]
	// まだ有効な最初のリクエストの位置を取得
	firstActive := 0
	// timestamp が cutoff 以下、つまり古すぎる間は firstActive を進める
	/**
	now = 09:01:30
	window = 1分
	cutoff = 09:00:30

	index 0: 09:00:10
	index 1: 09:00:30
	index 2: 09:00:40
	index 3: 09:01:05

	09:00:10 <= 09:00:30 なので古い -> firstActive++
	09:00:30 <= 09:00:30 なので古い -> firstActive++
	09:00:40 >  09:00:30 なので有効 -> stop
	*/
	for firstActive < len(timestamps) && !timestamps[firstActive].After(cutoff) {
		firstActive++
	}
	timestamps = timestamps[firstActive:]

	// System design interview points:
	// - Most accurate of the common rolling-window approaches because it stores exact request times.
	// - Memory usage grows with traffic: O(number of accepted requests inside the window).
	// - Distributed versions often use Redis sorted sets, trimming old timestamps and counting current ones atomically.
	if len(timestamps) >= l.limit {
		// limitを超えていればtimestampを戻す
		// limitの数を運用中に少なくした場合などにlimitよりも多くがlogに残る可能性があるので削った後のtimestampを入れ直す
		l.requests[key] = timestamps
		return false
	}

	timestamps = append(timestamps, now)
	// 古いものをすて、有効なタイムスタンプとこのリクエストのtimestampで上書きする
	l.requests[key] = timestamps
	return true
}
