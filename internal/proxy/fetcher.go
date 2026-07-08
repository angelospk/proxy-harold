package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalidURL     = errors.New("invalid URL")
	ErrInvalidScheme  = errors.New("URL scheme must be http or https")
	ErrResponseTooBig = errors.New("response exceeds maximum allowed size")
	ErrDisallowedHost = errors.New("URL targets a private or internal address")
)

// lookupIP resolves hostnames; a variable so tests can stub DNS.
// Note: validating at request time leaves a DNS-rebinding TOCTOU window
// between validation and fetch; accepted as out of scope for this proxy.
var lookupIP = net.LookupIP

// disallowedCIDRs are IPv4 special-use ranges not covered by the net.IP helpers
var disallowedCIDRs = mustParseCIDRs(
	"0.0.0.0/8",     // "this network" (RFC 1122)
	"100.64.0.0/10", // CGNAT (RFC 6598)
	"192.0.0.0/24",  // IETF protocol assignments (RFC 6890)
	"198.18.0.0/15", // benchmarking (RFC 2544)
	"240.0.0.0/4",   // reserved (RFC 1112)
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		nets = append(nets, n)
	}
	return nets
}

// Fetcher handles HTTP requests to remote URLs
type Fetcher struct {
	client  *http.Client
	maxSize int64

	// AllowPrivate permits fetching private/internal addresses.
	// Only intended for tests and trusted deployments.
	AllowPrivate bool
}

// NewFetcher creates a new URL fetcher with specified timeout and max response size
func NewFetcher(timeout time.Duration, maxSize int64) *Fetcher {
	f := &Fetcher{maxSize: maxSize}
	f.client = &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			// Re-validate every redirect target so a public URL cannot
			// bounce the proxy into a private/internal address
			return f.ValidateURL(req.URL.String())
		},
	}
	return f
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

	// Reject private/internal targets (SSRF guard)
	if !f.AllowPrivate {
		if err := checkHostAllowed(parsed.Hostname()); err != nil {
			return err
		}
	}

	return nil
}

// MaxSize returns the maximum allowed response size in bytes
func (f *Fetcher) MaxSize() int64 {
	return f.maxSize
}

// checkHostAllowed rejects hosts that point at private or internal networks
func checkHostAllowed(host string) error {
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("%w: %s", ErrDisallowedHost, host)
	}

	// Literal IP: check directly without DNS
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrInternal(ip) {
			return fmt.Errorf("%w: %s", ErrDisallowedHost, host)
		}
		return nil
	}

	ips, err := lookupIP(host)
	if err != nil {
		return fmt.Errorf("%w: cannot resolve %s: %v", ErrDisallowedHost, host, err)
	}
	for _, ip := range ips {
		if isPrivateOrInternal(ip) {
			return fmt.Errorf("%w: %s resolves to %s", ErrDisallowedHost, host, ip)
		}
	}
	return nil
}

// isPrivateOrInternal reports whether ip belongs to a loopback, private
// (RFC 1918 / IPv6 ULA), link-local (incl. 169.254.0.0/16 metadata range),
// multicast, unspecified, or other special-use network
func isPrivateOrInternal(ip net.IP) bool {
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	for _, n := range disallowedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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
