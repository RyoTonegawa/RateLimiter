package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ryotoneagwa/ratelimitter/internal/limiter"
)

type RateLimiters struct {
	TokenBucket    limiter.Limiter
	LeakingBucket  limiter.Limiter
	FixedWindow    limiter.Limiter
	SlidingLog     limiter.Limiter
	SlidingCounter limiter.Limiter
}

type Handler struct {
	limiters RateLimiters
}

type Response struct {
	Allowed   bool   `json:"allowed" example:"true"`
	Algorithm string `json:"algorithm" example:"token_bucket"`
	Message   string `json:"message" example:"request accepted"`
}

func NewHandler(limiters RateLimiters) *Handler {
	return &Handler{limiters: limiters}
}

// TokenBucket godoc
// @Summary Token Bucket方式のレートリミット
// @Description トークンを一定速度で補充し、リクエストごとに1トークンを消費します。
// @Description Pros: 平均レートを守りながら、バケット容量までの瞬間的なバーストを許可できます。API Gatewayやユーザー単位の制限でよく使われます。
// @Description Cons: 容量分のバーストは下流サービスにそのまま流れるため、瞬間負荷に弱い下流では注意が必要です。分散環境ではトークン数と最終補充時刻を原子的に更新する必要があります。
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /token-bucket [get]
func (h *Handler) TokenBucket(c *gin.Context) {
	h.handle(c, h.limiters.TokenBucket, "token_bucket")
}

// LeakingBucket godoc
// @Summary Leaking Bucket方式のレートリミット
// @Description バケットをキューのように扱い、一定速度で漏れ出す前提でリクエストを受け付けます。
// @Description Pros: 下流への流量を平滑化しやすく、急なスパイクを一定速度に近づけられます。
// @Description Cons: キューが大きいと遅延を隠してしまい、小さいとすぐ拒否します。このサンプルでは内部workerが固定間隔でキューをdrainしますが、実運用では永続化Queue、retry、timeout、バックプレッシャー設計も論点になります。
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /leaking-bucket [get]
func (h *Handler) LeakingBucket(c *gin.Context) {
	h.handle(c, h.limiters.LeakingBucket, "leaking_bucket")
}

// FixedWindowCounter godoc
// @Summary Fixed Window Counter方式のレートリミット
// @Description 固定長の時間窓ごとにリクエスト数をカウントします。
// @Description Pros: 実装が非常にシンプルで、キーごとにカウンターを1つ持てばよいため低コストです。Redis INCR + TTLでも実装しやすいです。
// @Description Cons: 窓の終端と次の窓の開始直後に連続で送ると、短時間で最大2窓分のリクエストを許してしまいます。境界バーストが代表的な弱点です。
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /fixed-window-counter [get]
func (h *Handler) FixedWindowCounter(c *gin.Context) {
	h.handle(c, h.limiters.FixedWindow, "fixed_window_counter")
}

// SlidingWindowLog godoc
// @Summary Sliding Window Log方式のレートリミット
// @Description リクエスト時刻をログとして保持し、現在時刻から見た直近ウィンドウ内の件数を正確に数えます。
// @Description Pros: 固定窓の境界問題を避けられ、ローリングウィンドウとして最も正確に制限できます。
// @Description Cons: 許可したリクエストのタイムスタンプを保持するため、トラフィック量やキー数が多いとメモリ使用量が増えます。分散環境ではRedis Sorted Setなどでtrim/count/addを原子的に扱う必要があります。
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /sliding-window-log [get]
func (h *Handler) SlidingWindowLog(c *gin.Context) {
	h.handle(c, h.limiters.SlidingLog, "sliding_window_log")
}

// SlidingWindowCounter godoc
// @Summary Sliding Window Counter方式のレートリミット
// @Description 現在の固定窓カウントと、前の固定窓カウントに時間割合の重みを掛けた値で、ローリングウィンドウを近似します。
// @Description Pros: Sliding Window Logよりメモリ効率が良く、キーごとに現在窓と前窓のカウンターだけで済みます。Fixed Windowより境界バーストを抑えやすいです。
// @Description Cons: あくまで近似のため、前窓のリクエスト分布によっては正確なLog方式より厳しく拒否したり、逆に緩く許可したりする可能性があります。
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /sliding-window-counter [get]
func (h *Handler) SlidingWindowCounter(c *gin.Context) {
	h.handle(c, h.limiters.SlidingCounter, "sliding_window_counter")
}

func (h *Handler) handle(c *gin.Context, limiter limiter.Limiter, algorithm string) {
	key := c.ClientIP()
	if limiter.Allow(key) {
		c.JSON(http.StatusOK, Response{
			Allowed:   true,
			Algorithm: algorithm,
			Message:   "request accepted",
		})
		return
	}

	c.JSON(http.StatusTooManyRequests, Response{
		Allowed:   false,
		Algorithm: algorithm,
		Message:   "rate limit exceeded",
	})
}
