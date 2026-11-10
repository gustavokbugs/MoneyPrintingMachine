// Package rss fetches and parses RSS 2.0 and Atom feeds that the user
// adds. Requests go only to public internet addresses: the dialer rejects
// loopback, private, link-local, and other special-purpose ranges so a feed
// URL cannot be used to reach internal services (SSRF).
package rss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

var (
	// ErrInvalidURL means the feed URL is malformed or not http(s).
	ErrInvalidURL = errors.New("feed URL must be an absolute http or https URL")
	// ErrBlockedAddress means the host resolves to a non-public address.
	ErrBlockedAddress = errors.New("feed host resolves to a private or reserved address")
	// ErrTooLarge means the response exceeded the size limit.
	ErrTooLarge = errors.New("feed is larger than the 2 MB limit")
)

// MaxFeedBytes bounds how much of a feed is read.
const MaxFeedBytes = 2 << 20

// ValidateURL checks that a feed URL is absolute http(s) without
// credentials. Address checks happen at connection time.
func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return nil, ErrInvalidURL
	}
	if len(raw) > 2048 {
		return nil, ErrInvalidURL
	}
	return u, nil
}

// isPublicIP reports whether ip is a globally routable unicast address.
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// Carrier-grade NAT (100.64.0.0/10) and benchmarking (198.18.0.0/15).
	for _, cidr := range []string{"100.64.0.0/10", "198.18.0.0/15", "192.0.0.0/24", "240.0.0.0/4"} {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// NewClient returns an HTTP client whose connections may only reach public
// addresses. allowPrivate exists for tests against local servers.
func NewClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlockedAddress
			}
			ip := net.ParseIP(host)
			if ip == nil || !isPublicIP(ip) {
				return ErrBlockedAddress
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil, // Never route feed requests through an environment proxy.
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          10,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return ErrInvalidURL
			}
			return nil
		},
	}
}

// FetchResult is the outcome of a conditional GET.
type FetchResult struct {
	NotModified  bool
	Feed         *Feed
	ETag         string
	LastModified string
}

// Fetcher downloads feeds.
type Fetcher struct {
	client    *http.Client
	userAgent string
}

// NewFetcher builds a Fetcher.
func NewFetcher(client *http.Client) *Fetcher {
	return &Fetcher{client: client, userAgent: "ContentFactory-FeedReader/1.0"}
}

// Fetch downloads and parses a feed, sending validators from the previous
// fetch so unchanged feeds cost no parsing.
func (f *Fetcher) Fetch(ctx context.Context, rawURL, etag, lastModified string) (FetchResult, error) {
	u, err := ValidateURL(rawURL)
	if err != nil {
		return FetchResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrBlockedAddress) {
			return FetchResult{}, ErrBlockedAddress
		}
		return FetchResult{}, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return FetchResult{NotModified: true, ETag: etag, LastModified: lastModified}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFeedBytes+1))
	if err != nil {
		return FetchResult{}, fmt.Errorf("read feed: %w", err)
	}
	if len(body) > MaxFeedBytes {
		return FetchResult{}, ErrTooLarge
	}
	feed, err := Parse(body)
	if err != nil {
		return FetchResult{}, err
	}
	return FetchResult{Feed: feed, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}, nil
}
