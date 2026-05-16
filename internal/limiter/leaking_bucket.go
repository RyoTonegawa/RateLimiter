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
	stopWorker   chan struct{}
	stopOnce     sync.Once
}

func NewLeakingBucketLimiter(capacity int, leakInterval time.Duration) *LeakingBucketLimiter {
	limiter := newLeakingBucketLimiter(capacity, leakInterval, time.Now)
	limiter.startWorker()
	return limiter
}

func newLeakingBucketLimiter(capacity int, leakInterval time.Duration, now func() time.Time) *LeakingBucketLimiter {
	return &LeakingBucketLimiter{
		capacity:     capacity,
		leakInterval: leakInterval,
		buckets:      make(map[string]*leakingBucketState),
		now:          now,
		stopWorker:   make(chan struct{}),
	}
}

func (l *LeakingBucketLimiter) startWorker() {
	ticker := time.NewTicker(l.leakInterval)

	go func() {
		for {
			select {
			case <-ticker.C:
				l.drainQueues()
			case <-l.stopWorker:
				ticker.Stop()
				return
			}
		}
	}()
}

func (l *LeakingBucketLimiter) Close() {
	l.stopOnce.Do(func() {
		close(l.stopWorker)
	})
}

func (l *LeakingBucketLimiter) drainQueues() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	for _, state := range l.buckets {
		l.drainState(state, now)
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
	// 図のLeaking Bucketは「Queueに入れて、別workerが固定レートで処理する」モデル。
	// そのため、Allowでは「満杯か確認してenqueueする」だけにする。
	// 実際のdequeueはstartWorkerから呼ばれるdrainQueuesが担当する。
	// テストではworkerを起動しないlimiterを作り、drainStateを明示的に呼ぶと固定時刻で検証できる。

	// System design interview points:
	// - Good when downstream systems need a smoother request rate instead of bursty traffic.
	// - Queue capacity is a backpressure decision: large queues hide spikes but increase latency.
	// - Production systems often pair this with an async worker that drains the queue at a fixed rate.
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

func (l *LeakingBucketLimiter) drainState(state *leakingBucketState, now time.Time) {
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
}
