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
// @Summary Token Bucket protected endpoint
// @Description Allows short bursts up to bucket capacity, then refills steadily over time.
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /token-bucket [get]
func (h *Handler) TokenBucket(c *gin.Context) {
	h.handle(c, h.limiters.TokenBucket, "token_bucket")
}

// LeakingBucket godoc
// @Summary Leaking Bucket protected endpoint
// @Description Smooths traffic by draining queued requests at a fixed rate.
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /leaking-bucket [get]
func (h *Handler) LeakingBucket(c *gin.Context) {
	h.handle(c, h.limiters.LeakingBucket, "leaking_bucket")
}

// FixedWindowCounter godoc
// @Summary Fixed Window Counter protected endpoint
// @Description Counts requests in fixed windows; simple but vulnerable to boundary bursts.
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /fixed-window-counter [get]
func (h *Handler) FixedWindowCounter(c *gin.Context) {
	h.handle(c, h.limiters.FixedWindow, "fixed_window_counter")
}

// SlidingWindowLog godoc
// @Summary Sliding Window Log protected endpoint
// @Description Stores request timestamps for an exact rolling-window limit.
// @Tags rate-limiters
// @Produce json
// @Success 200 {object} Response
// @Failure 429 {object} Response
// @Router /sliding-window-log [get]
func (h *Handler) SlidingWindowLog(c *gin.Context) {
	h.handle(c, h.limiters.SlidingLog, "sliding_window_log")
}

// SlidingWindowCounter godoc
// @Summary Sliding Window Counter protected endpoint
// @Description Approximates a rolling-window limit by weighting the previous fixed window.
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
