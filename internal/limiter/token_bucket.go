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
	/*
		l.mu.Lockの部分について
		mutexとは、複数のgoroutineから同じデータを同時に読み書きされないように守るためのロック。

		このAllow関数は、l.buckets、state.tokens、state.lastRefillという共有状態を更新する。
		もしLockがないと、同じkeyに対する複数リクエストが同時に入り、同じtoken数を読んでしまう可能性がある。

		例:
		- tokens = 1
		- request A が tokens を 1 と読む
		- request B も tokens を 1 と読む
		- AもBも許可され、本来1件しか通せないのに2件通ってしまう

		そのため、l.mu.Lock()でここから先の重要な処理を一つずつ実行させる。
		defer l.mu.Unlock()により、return trueでもreturn falseでも関数終了時に必ずLockを外す。
	*/
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	// バケットから各ユーザに対応するステートを取得。
	// Mapに対してこの書き方ができるのは便利だね
	state, ok := l.buckets[key]
	// バケット存在チェック
	if !ok {
		// 存在しない場合は新しくバケットを作成
		// structを作りそのポインタをstateに渡している
		state = &tokenBucketState{
			// キャパシティを入れるので最初は満タンからスタート。
			tokens:     l.capacity,
			lastRefill: now,
		}
		l.buckets[key] = state
	}
	// 最後に充填してから現在までの経過時間を求める
	elapsed := now.Sub(state.lastRefill).Seconds()
	// キャパシティ満タンか、トークン残量＋充填量どちらかキャパシティを超えない少ない方をトークン量とする
	// このとき、経過時間×再充填量が追加トークンとなるので、0.1s経過、1トークン充填ならば0.1トークンしか貯まらない
	state.tokens = min(l.capacity, state.tokens+elapsed*l.refillRate)
	// 最終充填時刻をいまにする
	state.lastRefill = now

	// System design interview points:
	// - Good when clients need occasional bursts while preserving a long-term average rate.
	// - Capacity controls maximum burst size; refill rate controls sustained throughput.
	// - Distributed implementations usually store token count + last-refill timestamp in Redis and update atomically with Lua.

	// 上記解説にある通り、再充填量は経過時間に依存するのでトークン枯渇が起きうる。
	if state.tokens < 1 {
		return false
	}
	// トークンを消費してtrueを返す
	state.tokens--
	return true
}
