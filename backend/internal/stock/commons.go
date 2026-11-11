package stock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Providers of stock media.
const (
	ProviderPexels  = "pexels"
	ProviderCommons = "commons"
)

// Commons searches Wikimedia Commons (no key needed) for photos whose
// license allows commercial reuse: public domain, CC0, CC BY, and CC BY-SA.
// Attribution licenses require crediting the author and license, which
// every returned clip carries.
type Commons struct {
	http *http.Client
	base string
	// gap spaces requests so a burst of searches stays within the API's
	// fair-use limits; last is guarded by mu.
	gap time.Duration
	// retryWait is the pause before retrying a rate-limited download.
	retryWait time.Duration
	mu        sync.Mutex
	last      time.Time
}

// NewCommons builds a Wikimedia Commons client.
func NewCommons(timeout time.Duration) *Commons {
	c := &Commons{base: "https://commons.wikimedia.org/w/api.php", gap: 700 * time.Millisecond, retryWait: 5 * time.Second}
	c.http = &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !allowedFileHost(req.URL) {
			return errors.New("unexpected redirect")
		}
		return nil
	}}
	return c
}

// userAgent identifies the tool as Wikimedia's API policy asks, without
// personal details.
const userAgent = "ContentFactory/0.1 (self-hosted video tool; Go net/http)"

// maxRetryWait bounds how long one rate-limited search waits to retry.
const maxRetryWait = 15 * time.Second

// get paces requests and retries once after a 429, honoring Retry-After
// up to maxRetryWait.
func (c *Commons) get(ctx context.Context, u string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("commons search: %w", err)
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		resp.Body.Close()
		if attempt > 0 {
			return nil, fmt.Errorf("wikimedia commons: %w", ErrRateLimited)
		}
		delay := 5 * time.Second
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
			delay = time.Duration(s) * time.Second
		}
		if delay > maxRetryWait {
			return nil, fmt.Errorf("wikimedia commons: %w", ErrRateLimited)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
}

// wait blocks until gap has passed since the previous request.
func (c *Commons) wait(ctx context.Context) error {
	c.mu.Lock()
	next := c.last.Add(c.gap)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	c.last = next
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Until(next)):
		return nil
	}
}

// Available is always true: Commons needs no key.
func (c *Commons) Available() bool { return c != nil }

type commonsResponse struct {
	Query struct {
		Pages map[string]struct {
			Title     string `json:"title"`
			ImageInfo []struct {
				ThumbURL    string `json:"thumburl"`
				ThumbWidth  int    `json:"thumbwidth"`
				ThumbHeight int    `json:"thumbheight"`
				Width       int    `json:"width"`
				Height      int    `json:"height"`
				Mime        string `json:"mime"`
				DescURL     string `json:"descriptionurl"`
				Meta        map[string]struct {
					Value any `json:"value"`
				} `json:"extmetadata"`
			} `json:"imageinfo"`
			PageID int64 `json:"pageid"`
			Index  int   `json:"index"`
		} `json:"pages"`
	} `json:"query"`
}

var licenseRe = regexp.MustCompile(`^(pd|cc0|cc-by-[0-9.]+|cc-by-sa-[0-9.]+)$`)

// commercialLicense reports whether a Commons license code allows
// commercial reuse (NC and ND licenses are not accepted on Commons, but
// unknown codes are rejected to be safe).
func commercialLicense(code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	return licenseRe.MatchString(code) || strings.HasPrefix(code, "pd-")
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// plainText strips HTML from Commons metadata values.
func plainText(v any) string {
	s, _ := v.(string)
	s = html.UnescapeString(tagRe.ReplaceAllString(s, ""))
	return strings.Join(strings.Fields(s), " ")
}

// Search returns photos for query in relevance order. Orientation is not
// filtered: images are cropped to the frame.
func (c *Commons) Search(ctx context.Context, query, _ string, width, height, perPage int) ([]Clip, error) {
	q := url.Values{}
	q.Set("action", "query")
	q.Set("format", "json")
	q.Set("generator", "search")
	q.Set("gsrsearch", query+" filetype:bitmap")
	q.Set("gsrnamespace", "6")
	q.Set("gsrlimit", fmt.Sprint(min(max(perPage, 1), 30)))
	q.Set("prop", "imageinfo")
	q.Set("iiprop", "url|size|mime|extmetadata")
	q.Set("iiextmetadatafilter", "License|LicenseShortName|Artist|ObjectName")
	q.Set("iiurlwidth", fmt.Sprint(max(width, height)))
	resp, err := c.get(ctx, c.base+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("commons search: HTTP %d", resp.StatusCode)
	}
	var cr commonsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&cr); err != nil {
		return nil, fmt.Errorf("decode commons search: %w", err)
	}
	type ranked struct {
		clip  Clip
		index int
	}
	var out []ranked
	for _, p := range cr.Query.Pages {
		if len(p.ImageInfo) == 0 {
			continue
		}
		ii := p.ImageInfo[0]
		u, err := url.Parse(ii.ThumbURL)
		if err != nil || !allowedFileHost(u) || (ii.Mime != "image/jpeg" && ii.Mime != "image/png") {
			continue
		}
		license := plainText(ii.Meta["License"].Value)
		if !commercialLicense(license) || min(ii.Width, ii.Height) < 720 {
			continue
		}
		author := plainText(ii.Meta["Artist"].Value)
		if len(author) > 120 {
			author = author[:120]
		}
		title := plainText(ii.Meta["ObjectName"].Value)
		if title == "" {
			title = strings.TrimSuffix(strings.TrimPrefix(p.Title, "File:"), ".jpg")
		}
		out = append(out, ranked{index: p.Index, clip: Clip{
			Provider: ProviderCommons, ID: p.PageID, PageURL: ii.DescURL, Author: author,
			License: plainText(ii.Meta["LicenseShortName"].Value), Title: title, Image: true,
			Width: ii.ThumbWidth, Height: ii.ThumbHeight, FileURL: ii.ThumbURL,
		}})
	}
	// Keep the search relevance order.
	sort.Slice(out, func(i, j int) bool { return out[i].index < out[j].index })
	clips := make([]Clip, 0, len(out))
	for _, r := range out {
		clips = append(clips, r.clip)
	}
	return preferRelevant(clips, query), nil
}

// preferRelevant keeps photos whose title shares a content word with the
// query, best matches first. Commons full-text search also matches
// descriptions, which often gives off-topic images, so photos with no
// shared word are dropped. Years, months, and filler words do not count:
// "September 1928" alone matches anything from that date.
func preferRelevant(clips []Clip, query string) []Clip {
	words := map[string]bool{}
	for _, w := range titleWords(query) {
		words[w] = true
	}
	type scored struct {
		clip  Clip
		score int
	}
	var keep []scored
	for _, c := range clips {
		score, seen := 0, map[string]bool{}
		for _, w := range titleWords(c.Title) {
			if words[w] && !seen[w] {
				seen[w] = true
				score++
			}
		}
		if score > 0 {
			keep = append(keep, scored{c, score})
		}
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].score > keep[j].score })
	out := make([]Clip, len(keep))
	for i, k := range keep {
		out[i] = k.clip
	}
	return out
}

// noMatchWords never make a photo relevant on their own.
var noMatchWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true, "into": true, "that": true, "this": true,
	"old": true, "new": true, "first": true, "photo": true, "image": true, "file": true, "view": true,
	"january": true, "february": true, "march": true, "april": true, "may": true, "june": true, "july": true,
	"august": true, "september": true, "october": true, "november": true, "december": true,
}

// titleWords returns lower-case content words of 3+ letters, singular,
// without numbers or filler words.
func titleWords(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) < 3 || noMatchWords[w] || strings.IndexFunc(w, unicode.IsLetter) < 0 {
			continue
		}
		out = append(out, strings.TrimSuffix(w, "s"))
	}
	return out
}

// Download stores the image under dir, cached by its URL hash. Image
// servers rate-limit bursts too, so downloads are paced like searches and
// retried once after a 429.
func (c *Commons) Download(ctx context.Context, clip Clip, dir string) (string, error) {
	if path, ok := Cached(clip, dir); ok {
		return path, nil // no request, so no pacing
	}
	for attempt := 0; ; attempt++ {
		if err := c.wait(ctx); err != nil {
			return "", err
		}
		path, err := download(ctx, c.http, clip, dir, userAgent)
		if err == nil || !errors.Is(err, ErrRateLimited) || attempt > 0 {
			return path, err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(c.retryWait):
		}
	}
}
