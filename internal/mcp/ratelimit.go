package mcp

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig holds configurable rate limit parameters.
type RateLimitConfig struct {
	DataRPS         float64
	DataBurst       int
	CleanupInterval time.Duration
	LimiterTTL      time.Duration
	MaxEntries      int
}

// DefaultRateLimitConfig returns production defaults: data 100 req/min burst 20.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		DataRPS:         100.0 / 60.0,
		DataBurst:       20,
		CleanupInterval: 5 * time.Minute,
		LimiterTTL:      10 * time.Minute,
		MaxEntries:      10000,
	}
}

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu             sync.Mutex
	limiters       map[string]*ipLimiter
	config         RateLimitConfig
	stopCh         chan struct{}
	trustedProxies []*net.IPNet
}

func NewRateLimiter(cfg RateLimitConfig, trustedProxies ...string) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*ipLimiter),
		config:   cfg,
		stopCh:   make(chan struct{}),
	}
	for _, cidr := range trustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ratelimit: invalid trusted proxy CIDR %q: %v\n", cidr, err)
			continue
		}
		rl.trustedProxies = append(rl.trustedProxies, network)
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

	if rl.config.MaxEntries > 0 && len(rl.limiters) >= rl.config.MaxEntries {
		rl.evictOldest()
	}

	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	rl.limiters[key] = &ipLimiter{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

func (rl *RateLimiter) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	for k, v := range rl.limiters {
		if oldestKey == "" || v.lastSeen.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.lastSeen
		}
	}
	if oldestKey != "" {
		delete(rl.limiters, oldestKey)
	}
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

// clientIP extracts the client IP, respecting X-Forwarded-For and X-Real-IP only
// when the direct connection IP matches a configured trusted proxy.
func (rl *RateLimiter) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	directIP := net.ParseIP(host)
	if directIP == nil {
		return host
	}

	if !rl.isTrustedProxy(directIP) {
		return host
	}

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
	return host
}

func (rl *RateLimiter) isTrustedProxy(ip net.IP) bool {
	if len(rl.trustedProxies) == 0 {
		return false
	}
	for _, network := range rl.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func RateLimitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.clientIP(r)

		if !rl.AllowData(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
