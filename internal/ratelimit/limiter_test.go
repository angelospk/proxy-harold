package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_AllowsRequestsUnderLimit(t *testing.T) {
	limiter := NewIPRateLimiter(10, 10) // 10 req/sec, burst of 10
	defer limiter.Cleanup()

	ip := "192.168.1.1"

	// Should allow up to burst limit
	for i := 0; i < 10; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("request %d should be allowed", i)
		}
	}
}

func TestRateLimiter_BlocksExcessiveRequests(t *testing.T) {
	limiter := NewIPRateLimiter(1, 5) // 1 req/sec, burst of 5
	defer limiter.Cleanup()

	ip := "192.168.1.1"

	// Use up burst
	for i := 0; i < 5; i++ {
		limiter.Allow(ip)
	}

	// Next request should be blocked
	if limiter.Allow(ip) {
		t.Error("request should be blocked after burst exhausted")
	}
}

func TestRateLimiter_DifferentIPsHaveSeparateLimits(t *testing.T) {
	limiter := NewIPRateLimiter(1, 2) // 1 req/sec, burst of 2
	defer limiter.Cleanup()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Exhaust ip1's limit
	limiter.Allow(ip1)
	limiter.Allow(ip1)

	// ip1 should be blocked
	if limiter.Allow(ip1) {
		t.Error("ip1 should be blocked")
	}

	// ip2 should still be allowed
	if !limiter.Allow(ip2) {
		t.Error("ip2 should be allowed")
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	limiter := NewIPRateLimiter(10, 1) // 10 req/sec, burst of 1
	defer limiter.Cleanup()

	ip := "192.168.1.1"

	// Use the one allowed request
	if !limiter.Allow(ip) {
		t.Error("first request should be allowed")
	}

	// Should be blocked
	if limiter.Allow(ip) {
		t.Error("second immediate request should be blocked")
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow(ip) {
		t.Error("request after refill should be allowed")
	}
}

func TestRateLimiter_Middleware(t *testing.T) {
	limiter := NewIPRateLimiter(1, 1) // Very strict: 1 req/sec, burst of 1
	defer limiter.Cleanup()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// First request should succeed
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec1.Code)
	}

	// Second immediate request should be rate limited
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "192.168.1.1:12345"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec2.Code)
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewIPRateLimiter(100, 100)
	defer limiter.Cleanup()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip := "192.168.1.1"
			limiter.Allow(ip)
		}(i)
	}
	wg.Wait()
	// Just testing for race conditions - if we get here without panic, it's good
}
