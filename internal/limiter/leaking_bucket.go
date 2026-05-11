package limiter

import (
	"sync"
	"time"
)

type leakingBucketState struct {
	waterLevel int
	lastLeak   time.Time
	queue      []leakingBucketRequest
}

type leakingBucketRequest struct {
	key        string
	enqueuedAt time.Time
}

type LeakingBucketLimiter struct {
	mu       sync.Mutex
	capacity int
	// 一つ漏れるまでの間隔
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
	// レースコンディション防止のために一つずつ処理する
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	state, ok := l.buckets[key]
	if !ok {
		state = &leakingBucketState{lastLeak: now}
		l.buckets[key] = state
	}
	// 小数点以下切り捨てで整数値を求める
	// 0.1 /1　= 0 , 3.5 / 1 = 3みたいな感じ
	leaked := int(now.Sub(state.lastLeak) / l.leakInterval)
	if leaked > 0 {
		// 最低ゼロとして現在のWaterLevelを求める
		// waterLevelは「仮想的な水量」だが、ここでは len(queue) と同期させている。
		drained := min(leaked, len(state.queue))
		// drainの範囲にある要素をQueueから削除する
		state.queue = state.queue[drained:]
		state.waterLevel = len(state.queue)
		//インターバル×Leakした分をlastLeakに追加して更新
		// 3個漏れたなら、lastLeakを3 interval分だけ進める
		// LastLeak = nowにしないのはleakedの計算時に小数点以下を切り捨てているため、
		// nowで小数点以下になる部分(単位が秒ならミリ秒)まで捨てると、次回以降のLeakが遅れるため。
		// 例: lastLeak=09:00:00, now=09:00:03.5, interval=1sなら leaked=3。
		// lastLeakをnow(09:00:03.5)にすると、切り捨てた0.5秒が失われる。
		// lastLeakを09:00:03まで進めておけば、残り0.5秒は次回のleak計算に持ち越される。
		state.lastLeak = state.lastLeak.Add(time.Duration(leaked) * l.leakInterval)
	}

	// System design interview points:
	// - Good when downstream systems need a smoother request rate instead of bursty traffic.
	// - Queue capacity is a backpressure decision: large queues hide spikes but increase latency.
	// - This sample models queue admission only; production systems often pair it with an async worker drain.
	// 満杯ならFalse
	if len(state.queue) >= l.capacity {
		return false
	}

	// ここでQueueにインサートする。
	// 本物のHTTP処理を後で実行するならrequestIDやpayloadをqueueに積み、
	// 別workerが一定間隔でdequeueして下流処理を実行する。
	// このAPIではqueueには受付イベントだけを積んでいる。
	state.queue = append(state.queue, leakingBucketRequest{
		key:        key,
		enqueuedAt: now,
	})
	state.waterLevel = len(state.queue)
	return true
}
