package mcp

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig holds configurable rate limit parameters.
type RateLimitConfig struct {
	AuthRPS         float64
	AuthBurst       int
	DataRPS         float64
	DataBurst       int
	CleanupInterval time.Duration
	LimiterTTL      time.Duration
}

// DefaultRateLimitConfig returns production defaults: auth 10 req/min burst 5, data 100 req/min burst 20.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		AuthRPS:         10.0 / 60.0,
		AuthBurst:       5,
		DataRPS:         100.0 / 60.0,
		DataBurst:       20,
		CleanupInterval: 5 * time.Minute,
		LimiterTTL:      10 * time.Minute,
	}
}

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter
	config   RateLimitConfig
	stopCh   chan struct{}
}

func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*ipLimiter),
		config:   cfg,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

func (rl *RateLimiter) getLimiter(key string, rps float64, burst int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if v, ok := rl.limiters[key]; ok {
		v.lastSeen = time.Now()
		return v.limiter
	}

	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	rl.limiters[key] = &ipLimiter{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

func (rl *RateLimiter) AllowAuth(key string) bool {
	return rl.getLimiter("auth:"+key, rl.config.AuthRPS, rl.config.AuthBurst).Allow()
}

func (rl *RateLimiter) AllowData(key string) bool {
	return rl.getLimiter("data:"+key, rl.config.DataRPS, rl.config.DataBurst).Allow()
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.config.LimiterTTL)
	for key, v := range rl.limiters {
		if v.lastSeen.Before(cutoff) {
			delete(rl.limiters, key)
		}
	}
}

// clientIP extracts the client IP, respecting X-Forwarded-For and X-Real-IP for reverse-proxy deployments.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx != -1 {
			xff = strings.TrimSpace(xff[:idx])
		}
		if xff != "" {
			return xff
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func RateLimitMiddleware(rl *RateLimiter, next http.Handler, classify func(r *http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		tier := classify(r)

		var allowed bool
		switch tier {
		case "auth":
			allowed = rl.AllowAuth(ip)
		default:
			allowed = rl.AllowData(ip)
		}

		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
