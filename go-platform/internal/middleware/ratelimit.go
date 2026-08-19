package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type RateLimiter struct {
	rate  rate.Limit
	burst int

	mu       sync.Mutex
	limiters sync.Map
}

func NewRateLimiter(r rate.Limit, b int) gin.HandlerFunc {
	rl := &RateLimiter{
		rate:  r,
		burst: b,
	}

	return func(c *gin.Context) {
		ip := c.ClientIP()

		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", time.Now().Add(time.Second).Format(time.RFC3339))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			c.Abort()
			return
		}

		remaining := int(limiter.Tokens())
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Next()
	}
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	if v, ok := rl.limiters.Load(key); ok {
		return v.(*rate.Limiter)
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if v, ok := rl.limiters.Load(key); ok {
		return v.(*rate.Limiter)
	}

	limiter := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters.Store(key, limiter)
	return limiter
}