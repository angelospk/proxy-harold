package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/harold/proxy-harold/internal/cache"
	"github.com/harold/proxy-harold/internal/proxy"
)

// mockCache implements cache.Cache for testing
type mockCache struct {
	data map[string][]byte
	ct   map[string]string
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string][]byte),
		ct:   make(map[string]string),
	}
}

func (m *mockCache) Get(url string) ([]byte, string, bool, error) {
	key := cache.GenerateCacheKey(url)
	data, exists := m.data[key]
	return data, m.ct[key], exists, nil
}

func (m *mockCache) Set(url string, data []byte, contentType string) error {
	key := cache.GenerateCacheKey(url)
	m.data[key] = data
	m.ct[key] = contentType
	return nil
}

func (m *mockCache) Delete(url string) error {
	key := cache.GenerateCacheKey(url)
	delete(m.data, key)
	delete(m.ct, key)
	return nil
}

func (m *mockCache) Close() error {
	return nil
}

func TestHandler_RequiresURLParameter(t *testing.T) {
	h := NewProxyHandler(newMockCache(), proxy.NewFetcher(10*time.Second, 10*1024*1024))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_RejectsInvalidURL(t *testing.T) {
	h := NewProxyHandler(newMockCache(), proxy.NewFetcher(10*time.Second, 10*1024*1024))

	req := httptest.NewRequest("GET", "/?url=javascript:alert(1)", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_FetchesAndCaches(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"test":"data"}`))
	}))
	defer server.Close()

	mockC := newMockCache()
	h := NewProxyHandler(mockC, proxy.NewFetcher(10*time.Second, 10*1024*1024))

	// First request - cache miss
	req := httptest.NewRequest("GET", "/?url="+server.URL, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != `{"test":"data"}` {
		t.Errorf("unexpected body: %s", body)
	}

	// Verify it indicates cache miss
	if rec.Header().Get("X-Cache") != "MISS" {
		t.Errorf("expected X-Cache: MISS, got %s", rec.Header().Get("X-Cache"))
	}

	// Verify data was cached
	key := cache.GenerateCacheKey(server.URL)
	if _, exists := mockC.data[key]; !exists {
		t.Error("expected data to be cached")
	}
}

func TestHandler_ReturnsCachedData(t *testing.T) {
	mockC := newMockCache()
	fetcher := proxy.NewFetcher(10*time.Second, 10*1024*1024)
	h := NewProxyHandler(mockC, fetcher)

	// Pre-populate cache
	testURL := "https://cached.example.com/data"
	mockC.Set(testURL, []byte(`{"cached":"response"}`), "application/json")

	req := httptest.NewRequest("GET", "/?url="+testURL, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if rec.Body.String() != `{"cached":"response"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}

	// Should indicate cache hit
	if rec.Header().Get("X-Cache") != "HIT" {
		t.Errorf("expected X-Cache: HIT, got %s", rec.Header().Get("X-Cache"))
	}
}

func TestHandler_SetsCORSHeaders(t *testing.T) {
	mockC := newMockCache()
	mockC.Set("https://example.com", []byte("data"), "text/plain")

	h := NewProxyHandler(mockC, proxy.NewFetcher(10*time.Second, 10*1024*1024))

	req := httptest.NewRequest("GET", "/?url=https://example.com", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	cors := rec.Header().Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("expected CORS header *, got %s", cors)
	}

	methods := rec.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("expected Access-Control-Allow-Methods header")
	}
}

func TestHandler_HandlesPreflight(t *testing.T) {
	h := NewProxyHandler(newMockCache(), proxy.NewFetcher(10*time.Second, 10*1024*1024))

	req := httptest.NewRequest("OPTIONS", "/?url=https://example.com", nil)
	req.Header.Set("Origin", "https://somesite.com")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", rec.Code)
	}

	cors := rec.Header().Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("expected CORS header *, got %s", cors)
	}
}

func TestHandler_ProxiesContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("fake png data"))
	}))
	defer server.Close()

	h := NewProxyHandler(newMockCache(), proxy.NewFetcher(10*time.Second, 10*1024*1024))

	req := httptest.NewRequest("GET", "/?url="+server.URL, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("expected Content-Type image/png, got %s", ct)
	}
}

func TestHandler_HandlesUpstreamErrors(t *testing.T) {
	// Use an invalid server that will refuse connections
	h := NewProxyHandler(newMockCache(), proxy.NewFetcher(1*time.Second, 10*1024*1024))

	req := httptest.NewRequest("GET", "/?url=http://localhost:59999/noexist", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rec.Code)
	}
}

// Helper to read response
func readBody(t *testing.T, resp *http.Response) string {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	return string(body)
}
