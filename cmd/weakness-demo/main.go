package main

import (
	"fmt"
	"time"

	"github.com/ryotoneagwa/ratelimitter/internal/limiter"
)

type scenario struct {
	name  string
	point string
	run   func() []string
}

func main() {
	scenarios := []scenario{
		{
			name:  "Token Bucket",
			point: "capacity分の瞬間バーストは許す。バーストが下流に刺さる設計では注意。",
			run: func() []string {
				now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
				l := limiter.NewTokenBucketLimiterWithClock(5, 1, func() time.Time { return now })
				return burst(l, "client", 6)
			},
		},
		{
			name:  "Leaking Bucket",
			point: "キュー容量までは受けるが、満杯になると漏れ出すまで拒否する。大きなキューは遅延を隠す。",
			run: func() []string {
				l := limiter.NewLeakingBucketLimiter(3, 100*time.Millisecond)
				defer l.Close()
				results := burst(l, "client", 4)
				time.Sleep(120 * time.Millisecond)
				results = append(results, "after worker drain: "+result(l.Allow("client")))
				return results
			},
		},
		{
			name:  "Fixed Window Counter",
			point: "窓の境界直前と直後に、短時間で最大2倍近いリクエストを許す。",
			run: func() []string {
				now := time.Date(2026, 5, 11, 9, 0, 59, 900*int(time.Millisecond), time.UTC)
				l := limiter.NewFixedWindowCounterLimiterWithClock(5, time.Minute, func() time.Time { return now })
				results := burst(l, "client", 5)
				now = time.Date(2026, 5, 11, 9, 1, 0, 100*int(time.Millisecond), time.UTC)
				results = append(results, burst(l, "client", 5)...)
				return results
			},
		},
		{
			name:  "Sliding Window Log",
			point: "正確だが、許可したリクエストごとにタイムスタンプを保存するため高トラフィックでメモリを使う。",
			run: func() []string {
				now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
				l := limiter.NewSlidingWindowLogLimiterWithClock(8, time.Minute, func() time.Time { return now })
				return burst(l, "client", 9)
			},
		},
		{
			name:  "Sliding Window Counter",
			point: "省メモリだが近似。前の固定窓に偏ったアクセスがあると、正確なログ方式より厳しく拒否することがある。",
			run: func() []string {
				now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
				counter := limiter.NewSlidingWindowCounterLimiterWithClock(5, time.Minute, func() time.Time { return now })
				exactLog := limiter.NewSlidingWindowLogLimiterWithClock(5, time.Minute, func() time.Time { return now })
				results := make([]string, 0, 8)
				for range 5 {
					counter.Allow("client")
					exactLog.Allow("client")
				}
				results = append(results, "previous fixed window: 5 requests at 09:00:00")
				now = time.Date(2026, 5, 11, 9, 1, 1, 0, time.UTC)
				results = append(results, "exact log at 09:01:01 #1: "+result(exactLog.Allow("client")))
				results = append(results, "counter at 09:01:01 #1: "+result(counter.Allow("client")))
				results = append(results, "exact log at 09:01:01 #2: "+result(exactLog.Allow("client")))
				results = append(results, "counter at 09:01:01 #2: "+result(counter.Allow("client")))
				return results
			},
		},
	}

	for _, s := range scenarios {
		fmt.Printf("\n== %s ==\n", s.name)
		fmt.Println(s.point)
		for i, result := range s.run() {
			fmt.Printf("%02d: %s\n", i+1, result)
		}
	}
}

func burst(l limiter.Limiter, key string, count int) []string {
	results := make([]string, 0, count)
	for range count {
		results = append(results, result(l.Allow(key)))
	}
	return results
}

func result(allowed bool) string {
	if allowed {
		return "allowed"
	}
	return "rejected"
}
