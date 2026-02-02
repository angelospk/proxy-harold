package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter manages rate limiters per IP address
type IPRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	done     chan struct{}
}

// NewIPRateLimiter creates a new rate limiter with specified rate (req/sec) and burst size
func NewIPRateLimiter(r float64, burst int) *IPRateLimiter {
	rl := &IPRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(r),
		burst:    burst,
		done:     make(chan struct{}),
	}

	// Start cleanup goroutine to remove stale limiters
	go rl.cleanupLoop()

	return rl
}

// getLimiter returns the rate limiter for the given IP, creating one if needed
func (rl *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.limiters[ip]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists = rl.limiters[ip]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[ip] = limiter
	return limiter
}

// Allow checks if a request from the given IP should be allowed
func (rl *IPRateLimiter) Allow(ip string) bool {
	return rl.getLimiter(ip).Allow()
}

// Cleanup stops the cleanup goroutine
func (rl *IPRateLimiter) Cleanup() {
	close(rl.done)
}

// cleanupLoop removes stale limiters periodically
func (rl *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			// In a production system, we'd track last access time
			// For now, we just clear very old entries if the map gets too large
			if len(rl.limiters) > 10000 {
				rl.limiters = make(map[string]*rate.Limiter)
			}
			rl.mu.Unlock()
		case <-rl.done:
			return
		}
	}
}

// Middleware returns an HTTP middleware that enforces rate limiting
func (rl *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		if !rl.Allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded","code":429}`))
			return
		}

		// Add rate limit headers
		limiter := rl.getLimiter(ip)
		w.Header().Set("X-RateLimit-Remaining", formatTokens(limiter.Tokens()))

		next.ServeHTTP(w, r)
	})
}

// extractIP gets the client IP from the request
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies like Cloudflare)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := len(xff); idx > 0 {
			for i, c := range xff {
				if c == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// formatTokens formats the remaining tokens as a string
func formatTokens(tokens float64) string {
	if tokens < 0 {
		return "0"
	}
	return string(rune('0' + int(tokens)%10))
}
