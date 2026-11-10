package rss

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const rssSample = `<?xml version="1.0"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/"><channel><title>Space &amp; News</title>
<item><title>NASA <b>Artemis</b> update</title><link>https://example.org/a</link><guid>a1</guid><pubDate>Mon, 09 Nov 2026 10:00:00 +0000</pubDate></item>
<item><title>Dublin Core dated</title><link>javascript:alert(1)</link><dc:date>2026-11-08T08:00:00Z</dc:date></item>
<item><title>No date entry</title><link>https://example.org/c</link></item>
<item><title></title><pubDate>Mon, 09 Nov 2026 10:00:00 +0000</pubDate></item>
</channel></rss>`

const atomSample = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>Atom Feed</title>
<entry><title>Mars rover finds clay</title><id>urn:1</id><updated>2026-11-07T12:00:00Z</updated>
<link rel="alternate" href="https://example.org/mars"/></entry>
</feed>`

func TestParseRSS(t *testing.T) {
	f, err := Parse([]byte(rssSample))
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "Space & News" || len(f.Items) != 2 {
		t.Fatalf("unexpected feed: %+v", f)
	}
	if f.Items[0].Title != "NASA Artemis update" || f.Items[0].Link != "https://example.org/a" {
		t.Fatalf("markup not cleaned: %+v", f.Items[0])
	}
	if f.Items[1].Link != "" {
		t.Fatalf("unsafe link kept: %q", f.Items[1].Link)
	}
	if f.Items[0].GUIDHash == f.Items[1].GUIDHash {
		t.Fatal("guid hashes must differ")
	}
}

func TestParseAtomAndInvalid(t *testing.T) {
	f, err := Parse([]byte(atomSample))
	if err != nil || len(f.Items) != 1 || f.Items[0].Link != "https://example.org/mars" {
		t.Fatalf("atom: %+v %v", f, err)
	}
	if _, err := Parse([]byte(`<html><body>nope</body></html>`)); !errors.Is(err, ErrNotAFeed) {
		t.Fatalf("expected not-a-feed, got %v", err)
	}
}

func TestValidateURL(t *testing.T) {
	for _, bad := range []string{"", "ftp://x/feed", "/relative", "https://user:pw@example.org/feed", "javascript:alert(1)"} {
		if _, err := ValidateURL(bad); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q: expected invalid, got %v", bad, err)
		}
	}
	if _, err := ValidateURL("https://example.org/feed.xml"); err != nil {
		t.Fatal(err)
	}
}

func TestIsPublicIP(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.5", "192.168.1.1", "172.16.3.4", "169.254.169.254", "::1", "fc00::1", "100.64.1.1", "0.0.0.0"} {
		if isPublicIP(net.ParseIP(ip)) {
			t.Errorf("%s must be blocked", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !isPublicIP(net.ParseIP(ip)) {
			t.Errorf("%s must be allowed", ip)
		}
	}
}

func TestFetchBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(rssSample)) }))
	defer srv.Close()
	f := NewFetcher(NewClient(5*time.Second, false))
	if _, err := f.Fetch(context.Background(), srv.URL, "", ""); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("expected blocked address, got %v", err)
	}
}

func TestFetchConditionalAndLimits(t *testing.T) {
	big := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if big {
			_, _ = w.Write([]byte(strings.Repeat("x", MaxFeedBytes+10)))
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(rssSample))
	}))
	defer srv.Close()
	f := NewFetcher(NewClient(5*time.Second, true))
	ctx := context.Background()

	res, err := f.Fetch(ctx, srv.URL, "", "")
	if err != nil || res.NotModified || res.ETag != `"v1"` || len(res.Feed.Items) != 2 {
		t.Fatalf("first fetch: %+v %v", res, err)
	}
	res, err = f.Fetch(ctx, srv.URL, `"v1"`, "")
	if err != nil || !res.NotModified {
		t.Fatalf("conditional fetch: %+v %v", res, err)
	}
	big = true
	if _, err := f.Fetch(ctx, srv.URL, "", ""); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected too large, got %v", err)
	}
}

func TestTokens(t *testing.T) {
	got := strings.Join(Tokens("The History of Space-Flight & AI"), ",")
	if got != "history,space,flight,ai" {
		t.Fatalf("tokens = %q", got)
	}
}
