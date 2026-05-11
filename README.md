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
