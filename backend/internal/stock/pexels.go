// Package stock finds and downloads free stock video clips through the
// official Pexels API (https://www.pexels.com/api/). Pexels content may be
// used commercially without attribution, but the API guidelines ask apps to
// credit Pexels and the creators, so every clip carries its credit.
package stock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrNoKey means PEXELS_API_KEY is not configured.
var ErrNoKey = errors.New("stock footage needs PEXELS_API_KEY (free at https://www.pexels.com/api/)")

// ErrRateLimited means the Pexels quota (200 requests/hour) is used up.
var ErrRateLimited = errors.New("pexels rate limit reached; try again later")

// MaxDownloadBytes bounds one clip download.
const MaxDownloadBytes = 120 << 20

// Clip is one stock video with the file chosen for download.
type Clip struct {
	ID        int64   `json:"id"`
	PageURL   string  `json:"pageUrl"`
	Author    string  `json:"author"`
	AuthorURL string  `json:"authorUrl"`
	Duration  float64 `json:"duration"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	FileURL   string  `json:"-"`
	FileID    int64   `json:"-"`
}

// Client calls the Pexels API.
type Client struct {
	key  string
	http *http.Client
	base string
}

// NewClient builds a client; an empty key disables it.
func NewClient(key string, timeout time.Duration) *Client {
	c := &Client{key: strings.TrimSpace(key), base: "https://api.pexels.com"}
	c.http = &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return errors.New("too many redirects")
		}
		if !allowedFileHost(req.URL) {
			return fmt.Errorf("redirect to unexpected host %q", req.URL.Hostname())
		}
		return nil
	}}
	return c
}

// Available reports whether a key is configured.
func (c *Client) Available() bool { return c != nil && c.key != "" }

type searchResponse struct {
	Videos []struct {
		ID       int64   `json:"id"`
		URL      string  `json:"url"`
		Duration float64 `json:"duration"`
		User     struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"user"`
		Files []struct {
			ID       int64  `json:"id"`
			FileType string `json:"file_type"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
			Link     string `json:"link"`
		} `json:"video_files"`
	} `json:"videos"`
}

// Search returns clips for query in the given orientation ("portrait" or
// "landscape"), each with the smallest MP4 file that still covers the
// target frame (or the largest available).
func (c *Client) Search(ctx context.Context, query, orientation string, width, height, perPage int) ([]Clip, error) {
	if !c.Available() {
		return nil, ErrNoKey
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("orientation", orientation)
	q.Set("size", "medium")
	q.Set("per_page", strconv.Itoa(min(max(perPage, 1), 40)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/videos/search?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.key)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels search: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.New("pexels rejected the API key")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("pexels search: HTTP %d", resp.StatusCode)
	}
	var sr searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode pexels search: %w", err)
	}
	var out []Clip
	for _, v := range sr.Videos {
		files := v.Files[:0:0]
		for _, f := range v.Files {
			u, err := url.Parse(f.Link)
			if err != nil || f.FileType != "video/mp4" || f.Width <= 0 || f.Height <= 0 || !allowedFileHost(u) {
				continue
			}
			files = append(files, f)
		}
		if len(files) == 0 || v.Duration <= 0 {
			continue
		}
		// Prefer the smallest file covering the frame; else the largest.
		sort.Slice(files, func(i, j int) bool { return files[i].Width*files[i].Height < files[j].Width*files[j].Height })
		pick := files[len(files)-1]
		for _, f := range files {
			if min(f.Width, f.Height) >= min(width, height) {
				pick = f
				break
			}
		}
		out = append(out, Clip{ID: v.ID, PageURL: v.URL, Author: v.User.Name, AuthorURL: v.User.URL, Duration: v.Duration,
			Width: pick.Width, Height: pick.Height, FileURL: pick.Link, FileID: pick.ID})
	}
	return out, nil
}

// allowedFileHost limits downloads to Pexels and the CDNs it serves
// files from, over HTTPS, so API data cannot point us at other hosts.
func allowedFileHost(u *url.URL) bool {
	if u == nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, suffix := range []string{"pexels.com", "vimeo.com", "vimeocdn.com", "akamaized.net"} {
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return true
		}
	}
	return false
}

// Download stores the clip's file under dir (cached by clip and file ID)
// and returns its path.
func (c *Client) Download(ctx context.Context, clip Clip, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create stock cache: %w", err)
	}
	out := filepath.Join(dir, fmt.Sprintf("pexels-%d-%d.mp4", clip.ID, clip.FileID))
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	u, err := url.Parse(clip.FileURL)
	if err != nil || !allowedFileHost(u) {
		return "", fmt.Errorf("clip %d has an unexpected file URL", clip.ID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download clip %d: %w", clip.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download clip %d: HTTP %d", clip.ID, resp.StatusCode)
	}
	if resp.ContentLength > MaxDownloadBytes {
		return "", fmt.Errorf("clip %d is too large (%d bytes)", clip.ID, resp.ContentLength)
	}
	f, err := os.CreateTemp(dir, "clip-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create clip file: %w", err)
	}
	tmp := f.Name()
	n, err := io.Copy(f, io.LimitReader(resp.Body, MaxDownloadBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > MaxDownloadBytes {
		err = fmt.Errorf("clip %d exceeds %d bytes", clip.ID, MaxDownloadBytes)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("download clip %d: %w", clip.ID, err)
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("store clip %d: %w", clip.ID, err)
	}
	return out, nil
}
