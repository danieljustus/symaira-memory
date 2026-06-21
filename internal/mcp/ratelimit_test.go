package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowAuth(t *testing.T) {
	cfg := RateLimitConfig{
		AuthRPS:         1.0,
		AuthBurst:       2,
		DataRPS:         10.0,
		DataBurst:       5,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	if !rl.AllowAuth("1.2.3.4") {
		t.Fatal("first auth request should be allowed")
	}
	if !rl.AllowAuth("1.2.3.4") {
		t.Fatal("second auth request should be allowed (burst=2)")
	}
	if rl.AllowAuth("1.2.3.4") {
		t.Fatal("third auth request should be denied (burst exhausted)")
	}
}

func TestRateLimiter_AllowData(t *testing.T) {
	cfg := RateLimitConfig{
		AuthRPS:         1.0,
		AuthBurst:       2,
		DataRPS:         1.0,
		DataBurst:       3,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		if !rl.AllowData("10.0.0.1") {
			t.Fatalf("data request %d should be allowed (burst=3)", i+1)
		}
	}
	if rl.AllowData("10.0.0.1") {
		t.Fatal("fourth data request should be denied (burst exhausted)")
	}
}

func TestRateLimiter_PerIPIsolation(t *testing.T) {
	cfg := RateLimitConfig{
		AuthRPS:         1.0,
		AuthBurst:       1,
		DataRPS:         1.0,
		DataBurst:       1,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	if !rl.AllowAuth("1.1.1.1") {
		t.Fatal("first request from IP 1.1.1.1 should be allowed")
	}
	if rl.AllowAuth("1.1.1.1") {
		t.Fatal("second request from IP 1.1.1.1 should be denied")
	}
	if !rl.AllowAuth("2.2.2.2") {
		t.Fatal("first request from IP 2.2.2.2 should be allowed (separate limiter)")
	}
}

func TestRateLimiter_AuthDataSeparation(t *testing.T) {
	cfg := RateLimitConfig{
		AuthRPS:         1.0,
		AuthBurst:       1,
		DataRPS:         1.0,
		DataBurst:       1,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	if !rl.AllowAuth("5.5.5.5") {
		t.Fatal("auth request should be allowed")
	}
	if rl.AllowAuth("5.5.5.5") {
		t.Fatal("second auth request should be denied")
	}
	if !rl.AllowData("5.5.5.5") {
		t.Fatal("data request should be allowed (separate bucket from auth)")
	}
}

func TestClientIP_XForwardedFor(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		AuthRPS:         10.0,
		AuthBurst:       10,
		DataRPS:         10.0,
		DataBurst:       10,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}, "127.0.0.0/8")
	defer rl.Stop()

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	if got := rl.clientIP(r); got != "203.0.113.50" {
		t.Fatalf("expected 203.0.113.50, got %s", got)
	}
}

func TestClientIP_XRealIP(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		AuthRPS:         10.0,
		AuthBurst:       10,
		DataRPS:         10.0,
		DataBurst:       10,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}, "127.0.0.0/8")
	defer rl.Stop()

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Real-IP", "198.51.100.17")
	if got := rl.clientIP(r); got != "198.51.100.17" {
		t.Fatalf("expected 198.51.100.17, got %s", got)
	}
}

func TestClientIP_RemoteAddr(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		AuthRPS:         10.0,
		AuthBurst:       10,
		DataRPS:         10.0,
		DataBurst:       10,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	})
	defer rl.Stop()

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.100:12345"
	if got := rl.clientIP(r); got != "192.168.1.100" {
		t.Fatalf("expected 192.168.1.100, got %s", got)
	}
}

func TestRateLimitMiddleware_Allowed(t *testing.T) {
	cfg := RateLimitConfig{
		AuthRPS:         10.0,
		AuthBurst:       10,
		DataRPS:         10.0,
		DataBurst:       10,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	classify := func(r *http.Request) string { return "data" }
	handler := RateLimitMiddleware(rl, inner, classify)

	req := httptest.NewRequest("GET", "/api/list", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRateLimitMiddleware_Denied(t *testing.T) {
	cfg := RateLimitConfig{
		AuthRPS:         1.0,
		AuthBurst:       1,
		DataRPS:         1.0,
		DataBurst:       1,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	classify := func(r *http.Request) string { return "data" }
	handler := RateLimitMiddleware(rl, inner, classify)

	req1 := httptest.NewRequest("GET", "/api/list", nil)
	req1.RemoteAddr = "10.0.0.2:9999"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest("GET", "/api/list", nil)
	req2.RemoteAddr = "10.0.0.2:9999"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") != "60" {
		t.Fatalf("expected Retry-After: 60, got %s", rec2.Header().Get("Retry-After"))
	}
}

func TestRateLimitMiddleware_AuthTier(t *testing.T) {
	cfg := RateLimitConfig{
		AuthRPS:         1.0,
		AuthBurst:       1,
		DataRPS:         10.0,
		DataBurst:       10,
		CleanupInterval: time.Hour,
		LimiterTTL:      time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	classify := func(r *http.Request) string {
		if r.URL.Path == "/api/token" {
			return "auth"
		}
		return "data"
	}
	handler := RateLimitMiddleware(rl, inner, classify)

	req1 := httptest.NewRequest("POST", "/api/token", nil)
	req1.RemoteAddr = "10.0.0.3:9999"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first auth request: expected 200, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest("POST", "/api/token", nil)
	req2.RemoteAddr = "10.0.0.3:9999"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second auth request: expected 429, got %d", rec2.Code)
	}
}

func TestDefaultRateLimitConfig(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	if cfg.AuthRPS <= 0 || cfg.AuthBurst <= 0 {
		t.Fatal("auth limits must be positive")
	}
	if cfg.DataRPS <= 0 || cfg.DataBurst <= 0 {
		t.Fatal("data limits must be positive")
	}
	if cfg.DataRPS <= cfg.AuthRPS {
		t.Fatal("data RPS should be higher than auth RPS")
	}
}
