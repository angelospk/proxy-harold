package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var (
	ErrInvalidURL     = errors.New("invalid URL")
	ErrInvalidScheme  = errors.New("URL scheme must be http or https")
	ErrResponseTooBig = errors.New("response exceeds maximum allowed size")
)

// Fetcher handles HTTP requests to remote URLs
type Fetcher struct {
	client  *http.Client
	maxSize int64
}

// NewFetcher creates a new URL fetcher with specified timeout and max response size
func NewFetcher(timeout time.Duration, maxSize int64) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: timeout,
			// Don't follow redirects automatically - let the proxy handle them
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
		maxSize: maxSize,
	}
}

// ValidateURL checks if the URL is valid and uses an allowed scheme
func (f *Fetcher) ValidateURL(rawURL string) error {
	if rawURL == "" {
		return ErrInvalidURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	// Only allow http and https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidScheme
	}

	// Must have a host
	if parsed.Host == "" {
		return ErrInvalidURL
	}

	return nil
}

// Fetch retrieves the content from the given URL
func (f *Fetcher) Fetch(rawURL string) (*http.Response, error) {
	if err := f.ValidateURL(rawURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set a user agent to avoid being blocked by some servers
	req.Header.Set("User-Agent", "ProxyHarold/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}

	// Check Content-Length if provided
	if resp.ContentLength > f.maxSize {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrResponseTooBig, resp.ContentLength, f.maxSize)
	}

	return resp, nil
}
