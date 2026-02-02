package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/harold/proxy-harold/internal/cache"
	"github.com/harold/proxy-harold/internal/proxy"
)

// Cache interface for dependency injection
type Cache interface {
	Get(url string) (data []byte, contentType string, found bool, err error)
	Set(url string, data []byte, contentType string) error
	Delete(url string) error
	Close() error
}

// ProxyHandler handles HTTP proxy requests
type ProxyHandler struct {
	cache   Cache
	fetcher *proxy.Fetcher
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(c Cache, f *proxy.Fetcher) *ProxyHandler {
	return &ProxyHandler{
		cache:   c,
		fetcher: f,
	}
}

// ErrorResponse represents a JSON error response
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for all responses
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Max-Age", "86400")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Get URL parameter
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		h.sendError(w, "missing required 'url' parameter", http.StatusBadRequest)
		return
	}

	// Validate URL
	if err := h.fetcher.ValidateURL(targetURL); err != nil {
		h.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check cache first
	if data, contentType, found, err := h.cache.Get(targetURL); err == nil && found {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Cache", "HIT")
		w.Write(data)
		return
	}

	// Fetch from upstream
	resp, err := h.fetcher.Fetch(targetURL)
	if err != nil {
		h.sendError(w, "failed to fetch URL: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.sendError(w, "failed to read response: "+err.Error(), http.StatusBadGateway)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Cache the response
	_ = h.cache.Set(targetURL, body, contentType)

	// Send response
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

// sendError sends a JSON error response
func (h *ProxyHandler) sendError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: message,
		Code:  code,
	})
}

// Ensure cache.BadgerCache implements Cache interface
var _ Cache = (*cache.BadgerCache)(nil)
