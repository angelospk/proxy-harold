package proxy

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestFetcher returns a fetcher that permits private addresses so tests
// can use httptest servers on 127.0.0.1
func newTestFetcher(timeout time.Duration, maxSize int64) *Fetcher {
	f := NewFetcher(timeout, maxSize)
	f.AllowPrivate = true
	return f
}

func TestFetcher_ValidatesURL(t *testing.T) {
	fetcher := newTestFetcher(10*time.Second, 10*1024*1024)

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

func TestFetcher_RejectsPrivateHosts(t *testing.T) {
	fetcher := NewFetcher(10*time.Second, 10*1024*1024)

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"localhost", "http://localhost/", true},
		{"localhost uppercase", "http://LOCALHOST/", true},
		{"localhost with port", "http://localhost:8080/", true},
		{"localhost subdomain", "http://foo.localhost/", true},
		{"loopback ipv4", "http://127.0.0.1/", true},
		{"loopback ipv4 range", "http://127.1.2.3/", true},
		{"loopback ipv6", "http://[::1]/", true},
		{"rfc1918 10.x", "http://10.0.0.5/", true},
		{"rfc1918 172.16.x", "http://172.16.0.1/", true},
		{"rfc1918 192.168.x", "http://192.168.1.1/", true},
		{"link-local metadata", "http://169.254.169.254/latest/meta-data/", true},
		{"link-local ipv6", "http://[fe80::1]/", true},
		{"ipv6 ula", "http://[fd00::1]/", true},
		{"unspecified", "http://0.0.0.0/", true},
		{"this-network 0/8", "http://0.1.2.3/", true},
		{"cgnat", "http://100.64.0.1/", true},
		{"benchmarking range", "http://198.18.0.1/", true},
		{"reserved 240/4", "http://240.0.0.1/", true},
		{"multicast", "http://224.0.0.1/", true},
		{"public ipv4", "http://8.8.8.8/", false},
		{"public ipv6", "http://[2001:4860:4860::8888]/", false},
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

func TestFetcher_RejectsHostResolvingToPrivateIP(t *testing.T) {
	// Stub DNS so the test never touches the network
	original := lookupIP
	defer func() { lookupIP = original }()
	lookupIP = func(host string) ([]net.IP, error) {
		switch host {
		case "internal.test":
			return []net.IP{net.ParseIP("10.0.0.5")}, nil
		case "public.test":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		case "mixed.test":
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("127.0.0.1")}, nil
		default:
			return nil, errors.New("no such host")
		}
	}

	fetcher := NewFetcher(10*time.Second, 10*1024*1024)

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"resolves to private", "http://internal.test/", true},
		{"resolves to public", "http://public.test/", false},
		{"resolves to mixed public+private", "http://mixed.test/", true},
		{"unresolvable host", "http://nowhere.test/", true},
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

func TestFetcher_RedirectToPrivateHostBlocked(t *testing.T) {
	fetcher := NewFetcher(10*time.Second, 10*1024*1024)

	// CheckRedirect must re-validate each redirect target
	redirectReq := httptest.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data/", nil)
	if err := fetcher.client.CheckRedirect(redirectReq, nil); err == nil {
		t.Error("expected redirect to private host to be rejected")
	}

	publicReq := httptest.NewRequest(http.MethodGet, "http://8.8.8.8/", nil)
	if err := fetcher.client.CheckRedirect(publicReq, nil); err != nil {
		t.Errorf("expected redirect to public host to be allowed, got %v", err)
	}
}

func TestFetcher_AllowPrivateSkipsGuard(t *testing.T) {
	fetcher := newTestFetcher(10*time.Second, 10*1024*1024)

	if err := fetcher.ValidateURL("http://127.0.0.1/"); err != nil {
		t.Errorf("expected AllowPrivate to permit loopback, got %v", err)
	}
}

func TestFetcher_FetchesURL(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"hello"}`))
	}))
	defer server.Close()

	fetcher := newTestFetcher(10*time.Second, 10*1024*1024)

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
	fetcher := newTestFetcher(50*time.Millisecond, 10*1024*1024)

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
	fetcher := newTestFetcher(10*time.Second, 1024) // 1KB max

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

			fetcher := newTestFetcher(10*time.Second, 10*1024*1024)
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
