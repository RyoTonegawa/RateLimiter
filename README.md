# RateLimitter

Ginで5種類のレートリミッターを実装した学習用APIです。

## Algorithms

- Token Bucket: `/token-bucket`
- Leaking Bucket: `/leaking-bucket`
- Fixed Window Counter: `/fixed-window-counter`
- Sliding Window Log: `/sliding-window-log`
- Sliding Window Counter: `/sliding-window-counter`

各エンドポイントはクライアントIP単位で制限します。実装コード内には、システムデザイン面接で説明しやすいように、各方式の強み・弱み・分散環境での論点をコメントとして残しています。

## Run

```bash
go run ./cmd/server
```

API:

```text
http://localhost:8080/token-bucket
http://localhost:8080/leaking-bucket
http://localhost:8080/fixed-window-counter
http://localhost:8080/sliding-window-log
http://localhost:8080/sliding-window-counter
```

Swagger UI:

```text
http://localhost:8080/swagger/index.html
```

## Test

```bash
go test ./...
```

## Weakness Demo

各アルゴリズムが苦手とするケースを短い時間窓で再現します。

```bash
go run ./cmd/weakness-demo
```

または:

```bash
bash scripts/weakness_demo.sh
```

確認できるポイント:

- Token Bucket: 容量分の瞬間バーストを許す
- Leaking Bucket: キュー満杯後は漏れ出すまで拒否する
- Fixed Window Counter: 窓境界の前後で短時間に2窓分を許す
- Sliding Window Log: 正確だがリクエスト時刻を保存するためメモリを使う
- Sliding Window Counter: 省メモリだが近似により厳しめに拒否することがある
