package limiter

import (
	"sync"
	"time"
)

type leakingBucketState struct {
	waterLevel int
	lastLeak   time.Time
}

type LeakingBucketLimiter struct {
	mu           sync.Mutex
	capacity     int
	leakInterval time.Duration
	buckets      map[string]*leakingBucketState
	now          func() time.Time
}

func NewLeakingBucketLimiter(capacity int, leakInterval time.Duration) *LeakingBucketLimiter {
	return &LeakingBucketLimiter{
		capacity:     capacity,
		leakInterval: leakInterval,
		buckets:      make(map[string]*leakingBucketState),
		now:          time.Now,
	}
}

func (l *LeakingBucketLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	state, ok := l.buckets[key]
	if !ok {
		state = &leakingBucketState{lastLeak: now}
		l.buckets[key] = state
	}

	leaked := int(now.Sub(state.lastLeak) / l.leakInterval)
	if leaked > 0 {
		state.waterLevel = max(0, state.waterLevel-leaked)
		state.lastLeak = state.lastLeak.Add(time.Duration(leaked) * l.leakInterval)
	}

	// System design interview points:
	// - Good when downstream systems need a smoother request rate instead of bursty traffic.
	// - Queue capacity is a backpressure decision: large queues hide spikes but increase latency.
	// - This sample models queue admission only; production systems often pair it with an async worker drain.
	if state.waterLevel >= l.capacity {
		return false
	}

	state.waterLevel++
	return true
}
