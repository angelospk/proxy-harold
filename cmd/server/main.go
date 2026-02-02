package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harold/proxy-harold/internal/cache"
	"github.com/harold/proxy-harold/internal/handler"
	"github.com/harold/proxy-harold/internal/proxy"
	"github.com/harold/proxy-harold/internal/ratelimit"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Configure logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Configuration from environment
	port := getEnv("PORT", "8080")
	cacheTTL := getEnvDuration("CACHE_TTL", 1*time.Hour)
	cacheDir := getEnv("CACHE_DIR", "./cache_data")
	rateLimit := getEnvFloat("RATE_LIMIT", 100) // requests per second
	rateBurst := getEnvInt("RATE_BURST", 200)   // burst size
	fetchTimeout := getEnvDuration("FETCH_TIMEOUT", 30*time.Second)
	maxResponseSize := getEnvInt64("MAX_RESPONSE_SIZE", 10*1024*1024) // 10MB

	log.Info().
		Str("port", port).
		Dur("cache_ttl", cacheTTL).
		Str("cache_dir", cacheDir).
		Float64("rate_limit", rateLimit).
		Int("rate_burst", rateBurst).
		Msg("Starting proxy server")

	// Initialize cache
	badgerCache, err := cache.NewBadgerCache(cacheDir, cacheTTL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize cache")
	}
	defer badgerCache.Close()

	// Initialize rate limiter
	limiter := ratelimit.NewIPRateLimiter(rateLimit, rateBurst)
	defer limiter.Cleanup()

	// Initialize fetcher
	fetcher := proxy.NewFetcher(fetchTimeout, maxResponseSize)

	// Initialize proxy handler
	proxyHandler := handler.NewProxyHandler(badgerCache, fetcher)

	// Build middleware chain
	var h http.Handler = proxyHandler
	h = limiter.Middleware(h)
	h = loggingMiddleware(h)

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle("/", h)
	mux.HandleFunc("/health", healthHandler)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("addr", server.Addr).Msg("Server listening")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server error")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	log.Info().Msg("Server stopped")
}

// loggingMiddleware logs all requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create response wrapper to capture status
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapped, r)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("url", r.URL.Query().Get("url")).
			Int("status", wrapped.statusCode).
			Dur("duration", time.Since(start)).
			Str("client_ip", r.RemoteAddr).
			Msg("Request handled")
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// healthHandler returns server health status
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// Environment helpers
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		var f float64
		if _, err := os.Stdin.Read(nil); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var i int
		for _, c := range value {
			if c >= '0' && c <= '9' {
				i = i*10 + int(c-'0')
			}
		}
		if i > 0 {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		var i int64
		for _, c := range value {
			if c >= '0' && c <= '9' {
				i = i*10 + int64(c-'0')
			}
		}
		if i > 0 {
			return i
		}
	}
	return defaultValue
}
