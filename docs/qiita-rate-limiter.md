# Go + Ginで5種類のレートリミッターを実装して理解する

## はじめに

システムデザイン面接でよく出てくるレートリミッターを、Go + Ginで実装しながら整理します。

この記事で扱うアルゴリズムは次の5つです。

- Token Bucket
- Leaking Bucket
- Fixed Window Counter
- Sliding Window Log
- Sliding Window Counter

実装の目的は、ライブラリを使うことではなく、各方式の内部状態、苦手なケース、面接で説明すべきPros/Consを理解することです。

## API構成

Ginで各アルゴリズムを別エンドポイントとして公開します。

```text
GET /token-bucket
GET /leaking-bucket
GET /fixed-window-counter
GET /sliding-window-log
GET /sliding-window-counter
GET /swagger/index.html
```

各リミッターは共通して次のinterfaceを満たします。

```go
type Limiter interface {
	Allow(key string) bool
}
```

`key` にはクライアントIPなどを渡します。

```go
key := c.ClientIP()
```

つまり、IPごとに別々のレートリミット状態を持つ設計です。

## Token Bucket

Token Bucketは、バケットにトークンを貯めておき、リクエストごとに1トークンを消費する方式です。

```go
func (l *TokenBucketLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	state, ok := l.buckets[key]
	if !ok {
		state = &tokenBucketState{
			tokens:     l.capacity,
			lastRefill: now,
		}
		l.buckets[key] = state
	}

	elapsed := now.Sub(state.lastRefill).Seconds()
	state.tokens = min(l.capacity, state.tokens+elapsed*l.refillRate)
	state.lastRefill = now

	if state.tokens < 1 {
		return false
	}

	state.tokens--
	return true
}
```

重要なのはこの行です。

```go
state.tokens = min(l.capacity, state.tokens+elapsed*l.refillRate)
```

経過時間に応じてトークンを補充します。ただし、`capacity` を超えないように `min` で上限をかけます。

例えば `refillRate = 1` は「1秒に1トークン補充」という意味です。0.1秒しか経っていなければ、0.1トークンしか補充されません。

### Mutexの役割

`Allow` は内部で `map` や `tokens` を更新します。

```go
l.buckets[key] = state
state.tokens--
```

複数リクエストが同時に来ると、同じ `tokens = 1` を複数goroutineが読んで、両方通してしまう可能性があります。

そのため、`sync.Mutex` でクリティカルセクションを守ります。

```go
l.mu.Lock()
defer l.mu.Unlock()
```

`defer` を使うことで、`return true` でも `return false` でも関数終了時に必ずロックが外れます。

### Pros

- 平均レートを守りながら、容量分の瞬間バーストを許可できる
- API Gatewayやユーザー単位の制限で使いやすい
- `capacity` が最大バースト量、`refillRate` が持続的な処理量になる

### Cons

- 容量分のバーストは下流にそのまま流れる
- 下流が瞬間負荷に弱い場合は注意が必要
- 分散環境ではトークン数と最終補充時刻をRedisなどで原子的に更新する必要がある

面接ではこう説明できます。

```text
Token Bucket allows short bursts up to the bucket capacity while enforcing a long-term average rate.
```

## Leaking Bucket

Leaking Bucketは、バケットに溜まったリクエストを一定速度で漏らす方式です。

今回の実装では、内部にQueueを持たせています。

```go
type leakingBucketState struct {
	waterLevel int
	lastLeak   time.Time
	queue      []leakingBucketRequest
}
```

`waterLevel` は水量、`queue` はバケット内に溜まっているリクエストを表します。

### leak数の計算

```go
leaked := int(now.Sub(state.lastLeak) / l.leakInterval)
```

`time.Duration / time.Duration` は整数除算です。

```text
0.1秒 / 1秒 = 0
1.5秒 / 1秒 = 1
3.5秒 / 1秒 = 3
```

つまり、小数点以下は切り捨てられます。

### Queueからdequeueする

```go
drained := min(leaked, len(state.queue))
state.queue = state.queue[drained:]
state.waterLevel = len(state.queue)
```

`leaked` は「時間的に漏らせる数」です。

ただし、Queueにそれより少ない件数しか入っていない場合があります。そこで `min(leaked, len(state.queue))` を使います。

例えば:

```text
leaked = 5
queue = [A, B]
```

なら、実際にdequeueできるのは2件だけです。

```go
drained := min(5, 2) // 2
```

`state.queue = state.queue[drained:]` は、先頭から `drained` 件を削除するスライス操作です。

```text
[A, B, C, D]
drained = 2
=> [C, D]
```

### lastLeakをnowにしない理由

```go
state.lastLeak = state.lastLeak.Add(time.Duration(leaked) * l.leakInterval)
```

ここで `state.lastLeak = now` にしないのが重要です。

例えば:

```text
lastLeak = 09:00:00
now = 09:00:03.5
leakInterval = 1秒
```

このとき:

```text
leaked = 3
```

0.5秒ぶんは切り捨てられます。

もし `lastLeak = now` にすると、切り捨てた0.5秒まで失われます。すると次回のleakが遅れます。

そのため、実際に漏れた `3 * interval` ぶんだけ進めます。

```text
lastLeak = 09:00:00 + 3秒
         = 09:00:03
```

これで残り0.5秒は次回の計算に持ち越されます。

### enqueueする場所

満杯でなければ、ここでQueueに入れます。

```go
if len(state.queue) >= l.capacity {
	return false
}

state.queue = append(state.queue, leakingBucketRequest{
	key:        key,
	enqueuedAt: now,
})
state.waterLevel = len(state.queue)
return true
```

今回の実装では、`NewLeakingBucketLimiter` が内部workerを起動し、`leakInterval` ごとにQueueをdrainします。

つまり通常実行時は:

```text
Allow() は enqueue する
worker は fixed rate で dequeue する
```

という形です。

ただし、このサンプルのQueueにはHTTPリクエスト本体や下流処理payloadは入れていません。学習用に受付イベントを積み、workerが固定レートで取り出すところまでを表現しています。

本番で本当に下流への流量を平準化したいなら、外部QueueとConsumerを使う構成にします。

```text
Client
  ↓
API Server
  ↓ enqueue
External Queue
  ↓ fixed-rate Consumer
Backend API
```

英語では次のように説明できます。

```text
Leaking Bucket smooths out traffic spikes by queueing requests and draining them at a fixed rate.
```

### Pros

- Peakを平準化できる
- 下流へのリクエスト量を一定に近づけられる
- Queue capacityによりバックプレッシャーを表現できる

### Cons

- Queueに溜めるためレイテンシが増える
- Queueが大きすぎると遅延やメモリ使用量が増える
- Queueが小さすぎるとすぐ429になる
- インメモリQueueは複数台構成で共有できない
- 本番で平準化するならSQS、Kafka、RabbitMQ、Redis Streamsなど外部Queueが必要になる

## Fixed Window Counter

Fixed Window Counterは、固定長の時間窓ごとにリクエスト数を数える方式です。

```go
now := l.now()
state, ok := l.counters[key]
if !ok || now.Sub(state.windowStart) >= l.window {
	state = &fixedWindowState{
		windowStart: now.Truncate(l.window),
		count:       0,
	}
	l.counters[key] = state
}
```

`now.Truncate(l.window)` は、現在時刻をwindow単位で切り捨てます。

例えば `window = 1分` の場合:

```text
09:03:27 -> 09:03:00
09:03:59 -> 09:03:00
09:04:00 -> 09:04:00
```

つまり:

```text
09:03:00 - 09:03:59
09:04:00 - 09:04:59
```

という固定窓でカウントします。

許可判定はシンプルです。

```go
if state.count >= l.limit {
	return false
}

state.count++
return true
```

### Pros

- 実装が非常にシンプル
- keyごとに1つのカウンターを持てばよい
- Redisなら `INCR` + `TTL` で実装しやすい

### Cons

- 窓の境界でバーストが起きる

例えば `limit = 5/min` の場合:

```text
09:00:59.900 に5リクエスト
09:01:00.100 に5リクエスト
```

この2つは別windowなので、短時間に10リクエスト許してしまいます。

面接ではこれをBoundary Burstと説明します。

```text
Fixed Window Counter is simple and cheap, but it can allow boundary bursts around window transitions.
```

## Sliding Window Log

Sliding Window Logは、リクエスト時刻をすべて保存し、現在時刻から見た直近window内の件数を正確に数えます。

```go
now := l.now()
cutoff := now.Add(-l.window)
timestamps := l.requests[key]
```

`cutoff` は「これより古いtimestampは数えない」という境界です。

```text
now = 09:01:30
window = 1分
cutoff = 09:00:30
```

次に古いtimestampを捨てます。

```go
firstActive := 0
for firstActive < len(timestamps) && !timestamps[firstActive].After(cutoff) {
	firstActive++
}
timestamps = timestamps[firstActive:]
```

`!timestamps[firstActive].After(cutoff)` は、ざっくり言えば:

```text
timestamp <= cutoff
```

です。

つまり、cutoff以前の古いtimestampを先頭から飛ばしています。

その後、残ったtimestamp数で判定します。

```go
if len(timestamps) >= l.limit {
	l.requests[key] = timestamps
	return false
}

timestamps = append(timestamps, now)
l.requests[key] = timestamps
return true
```

拒否時にも `l.requests[key] = timestamps` をしています。

これは、新しいtimestampを追加しているのではなく、古いtimestampを削除した後のsliceを保存し直すためです。

保存しないと、期限切れtimestampがmap内に残り続けます。

### limitより多く残るケース

通常の `Allow()` 経由で、かつlockが効いていて、limitが途中変更されないなら、active window内の件数は基本的に `limit` を超えません。

```text
len = 4 -> append -> len = 5
len = 5 -> reject
```

ただし、次のような場合には `limit` より多く残る可能性があります。

- 過去のバグで多くappendされた
- limitを途中で小さくした
- lockがなく、並行アクセスで複数goroutineが同時にappendした
- テストで内部状態を直接作った

実装上 `>=` にしているのは、`limit` ちょうどの状態で次のリクエストを拒否するためです。

### Pros

- 直近windowを正確に判定できる
- Fixed Windowの境界バーストを避けられる

### Cons

- 許可したリクエスト時刻を全部保存するためメモリを使う
- 高トラフィック・高cardinalityのkeyでは重くなる
- 分散環境ではRedis Sorted Setなどでtrim/count/addを原子的に扱う必要がある

## Sliding Window Counter

Sliding Window Counterは、Sliding Window Logを軽量化した近似方式です。

現在の固定窓カウントと、前の固定窓カウントに重みを掛けた値で推定します。

```go
elapsed := now.Sub(state.currentWindowStart)
previousWeight := float64(l.window-elapsed) / float64(l.window)
estimatedCount := float64(state.currentCount) + float64(state.previousCount)*previousWeight
```

例えばwindowが1分で、現在window開始から15秒経過しているなら:

```text
previousWeight = 45秒 / 60秒 = 0.75
```

前windowのカウントの75%を、まだ現在のrolling windowに含まれているものとして扱います。

### Pros

- Sliding Window Logよりメモリ効率が良い
- keyごとに現在windowと前windowのカウンターだけで済む
- Fixed Windowより境界バーストを抑えやすい

### Cons

- 近似なので誤差が出る
- 前windowのリクエスト分布によっては、正確なLog方式より厳しく拒否することがある
- 逆に緩く許可するケースもあり得る

面接ではこう説明できます。

```text
Sliding Window Counter is a memory-efficient approximation of Sliding Window Log.
It reduces boundary bursts but may be inaccurate because it assumes requests in the previous window were evenly distributed.
```

## 5方式の比較

| Algorithm | Pros | Cons |
|---|---|---|
| Token Bucket | 平均レートを守りつつバースト許可 | 容量分の瞬間負荷が下流に流れる |
| Leaking Bucket | スパイクを平準化しやすい | Queue遅延・メモリ・分散Queueが論点 |
| Fixed Window Counter | 実装が簡単で低コスト | 境界バースト |
| Sliding Window Log | 正確なrolling window | メモリ使用量が多い |
| Sliding Window Counter | 省メモリで境界バーストを軽減 | 近似なので誤差がある |

## 本番設計での注意

単一プロセス内の `map` やQueueは、複数台構成では共有されません。

```text
Server A のmap
Server B のmap
Server C のmap
```

は別々です。

本番では目的に応じて設計を分けます。

即時にallow/rejectしたい場合:

```text
API Server
  ↓
Redis
  - token count
  - last refill time
  - counters
  - sorted sets
```

下流処理を本当に平準化したい場合:

```text
API Server
  ↓ enqueue
External Queue
  ↓ fixed-rate workers
Backend Service
```

候補としては:

- Redis
- Redis Streams
- SQS
- Kafka
- RabbitMQ
- Pub/Sub

などがあります。

## まとめ

レートリミッターは「どれが一番良いか」ではなく、目的によって選ぶものです。

- バーストを少し許したいならToken Bucket
- 下流への流量を平準化したいならLeaking Bucket
- とにかく簡単に実装したいならFixed Window Counter
- 正確なrolling windowが必要ならSliding Window Log
- 省メモリでrolling windowに近づけたいならSliding Window Counter

システムデザイン面接では、アルゴリズム名だけでなく、次を説明できると強いです。

- 何を状態として持つか
- 時間をどう扱うか
- どんなバーストを許すか
- メモリ使用量はどれくらいか
- 分散環境ではどこに状態を置くか
- 下流を守るにはQueueやWorkerが必要か

実装してみると、各方式の違いはかなり具体的に見えるようになります。
