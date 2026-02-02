package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetcher_ValidatesURL(t *testing.T) {
	fetcher := NewFetcher(10*time.Second, 10*1024*1024)

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com", false},
		{"empty url", "", true},
		{"invalid scheme", "ftp://example.com", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"data scheme", "data:text/html,<h1>test</h1>", true},
		{"no scheme", "example.com", true},
		{"relative path", "/path/to/resource", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fetcher.ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestFetcher_FetchesURL(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"hello"}`))
	}))
	defer server.Close()

	fetcher := NewFetcher(10*time.Second, 10*1024*1024)

	resp, err := fetcher.Fetch(server.URL)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"message":"hello"}` {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestFetcher_RespectsTimeout(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("slow response"))
	}))
	defer server.Close()

	// Use very short timeout
	fetcher := NewFetcher(50*time.Millisecond, 10*1024*1024)

	_, err := fetcher.Fetch(server.URL)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestFetcher_RejectsTooLargeResponse(t *testing.T) {
	// Create a server returning large content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999999999")
		w.Write([]byte("start of large response"))
	}))
	defer server.Close()

	// Use small max size
	fetcher := NewFetcher(10*time.Second, 1024) // 1KB max

	_, err := fetcher.Fetch(server.URL)
	if err == nil {
		t.Error("expected size limit error")
	}
}

func TestFetcher_PreservesContentType(t *testing.T) {
	tests := []struct {
		contentType string
	}{
		{"application/json"},
		{"text/html; charset=utf-8"},
		{"image/png"},
		{"application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.Write([]byte("content"))
			}))
			defer server.Close()

			fetcher := NewFetcher(10*time.Second, 10*1024*1024)
			resp, err := fetcher.Fetch(server.URL)
			if err != nil {
				t.Fatalf("Fetch failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.Header.Get("Content-Type") != tt.contentType {
				t.Errorf("expected Content-Type %q, got %q", tt.contentType, resp.Header.Get("Content-Type"))
			}
		})
	}
}
