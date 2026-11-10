// Package youtube is an adapter for the official YouTube Data API v3 and
// Google OAuth 2.0. It never scrapes pages. Every Data API call reserves
// its documented quota cost before it is sent.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Quota costs in units, from the YouTube Data API quota table.
const (
	CostSearch   = 100
	CostVideos   = 1
	CostChannels = 1
)

// DefaultDataBaseURL is the production Data API endpoint.
const DefaultDataBaseURL = "https://www.googleapis.com/youtube/v3"

var (
	// ErrNotConfigured means no API key or OAuth client is configured.
	ErrNotConfigured = errors.New("YouTube integration is not configured")
	// ErrQuotaBudget means the local daily quota budget would be exceeded.
	ErrQuotaBudget = errors.New("YouTube daily quota budget reached")
	// ErrQuotaExceeded means Google rejected the call for quota reasons.
	ErrQuotaExceeded = errors.New("YouTube API quota exceeded")
	// ErrUnauthorized means the credentials were rejected.
	ErrUnauthorized = errors.New("YouTube rejected the credentials")
	// ErrUnavailable is a transient transport or server failure.
	ErrUnavailable = errors.New("YouTube API unavailable")
)

// APIError carries a Google error response.
type APIError struct {
	Status  int
	Reason  string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("YouTube API error %d (%s): %s", e.Status, e.Reason, e.Message)
}

// Quota reserves units against the daily budget before a call is made.
type Quota interface {
	Reserve(ctx context.Context, units int) error
}

// DataClient calls the YouTube Data API.
type DataClient struct {
	http    *http.Client
	baseURL string
	apiKey  string
	quota   Quota
}

// NewDataClient builds a DataClient. apiKey may be empty when only
// OAuth-authenticated calls are used.
func NewDataClient(httpClient *http.Client, baseURL, apiKey string, quota Quota) *DataClient {
	if baseURL == "" {
		baseURL = DefaultDataBaseURL
	}
	return &DataClient{http: httpClient, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, quota: quota}
}

// HasAPIKey reports whether key-authenticated (public data) calls work.
func (c *DataClient) HasAPIKey() bool { return c.apiKey != "" }

// SearchParams selects recent videos for a query.
type SearchParams struct {
	Query          string
	RegionCode     string // ISO 3166-1 alpha-2.
	Language       string // ISO 639-1, e.g. "en".
	PublishedAfter time.Time
	MaxResults     int
}

// SearchVideoIDs returns IDs of the most viewed videos matching the query
// published after PublishedAfter. Cost: 100 units.
func (c *DataClient) SearchVideoIDs(ctx context.Context, p SearchParams) ([]string, error) {
	if c.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if p.MaxResults <= 0 || p.MaxResults > 50 {
		p.MaxResults = 50
	}
	q := url.Values{
		"part":           {"id"},
		"type":           {"video"},
		"order":          {"viewCount"},
		"q":              {p.Query},
		"maxResults":     {strconv.Itoa(p.MaxResults)},
		"publishedAfter": {p.PublishedAfter.UTC().Format(time.RFC3339)},
	}
	if p.RegionCode != "" {
		q.Set("regionCode", p.RegionCode)
	}
	if p.Language != "" {
		q.Set("relevanceLanguage", p.Language)
	}
	var resp struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
		} `json:"items"`
	}
	if err := c.get(ctx, "/search", q, CostSearch, "", &resp); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(resp.Items))
	for _, it := range resp.Items {
		if it.ID.VideoID != "" {
			ids = append(ids, it.ID.VideoID)
		}
	}
	return ids, nil
}

// Video holds public statistics for one video.
type Video struct {
	ID           string
	ChannelID    string
	PublishedAt  time.Time
	ViewCount    int64
	LikeCount    int64 // 0 when hidden.
	CommentCount int64 // 0 when disabled.
}

// Videos returns statistics for up to 50 videos. Cost: 1 unit.
func (c *DataClient) Videos(ctx context.Context, ids []string) ([]Video, error) {
	if c.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 50 {
		return nil, errors.New("at most 50 video IDs per request")
	}
	var resp struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				ChannelID   string    `json:"channelId"`
				PublishedAt time.Time `json:"publishedAt"`
			} `json:"snippet"`
			Statistics struct {
				ViewCount    string `json:"viewCount"`
				LikeCount    string `json:"likeCount"`
				CommentCount string `json:"commentCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	q := url.Values{"part": {"snippet,statistics"}, "id": {strings.Join(ids, ",")}, "maxResults": {"50"}}
	if err := c.get(ctx, "/videos", q, CostVideos, "", &resp); err != nil {
		return nil, err
	}
	out := make([]Video, 0, len(resp.Items))
	for _, it := range resp.Items {
		out = append(out, Video{
			ID: it.ID, ChannelID: it.Snippet.ChannelID, PublishedAt: it.Snippet.PublishedAt,
			ViewCount: parseCount(it.Statistics.ViewCount), LikeCount: parseCount(it.Statistics.LikeCount),
			CommentCount: parseCount(it.Statistics.CommentCount),
		})
	}
	return out, nil
}

// ChannelInfo holds public information about a channel.
type ChannelInfo struct {
	ID               string
	Title            string
	CustomURL        string // The channel handle, e.g. "@example".
	SubscriberCount  int64
	HiddenSubscriber bool
	VideoCount       int64
}

type channelsResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title     string `json:"title"`
			CustomURL string `json:"customUrl"`
		} `json:"snippet"`
		Statistics struct {
			SubscriberCount       string `json:"subscriberCount"`
			HiddenSubscriberCount bool   `json:"hiddenSubscriberCount"`
			VideoCount            string `json:"videoCount"`
		} `json:"statistics"`
	} `json:"items"`
}

func (r channelsResponse) infos() []ChannelInfo {
	out := make([]ChannelInfo, 0, len(r.Items))
	for _, it := range r.Items {
		out = append(out, ChannelInfo{
			ID: it.ID, Title: it.Snippet.Title, CustomURL: it.Snippet.CustomURL,
			SubscriberCount:  parseCount(it.Statistics.SubscriberCount),
			HiddenSubscriber: it.Statistics.HiddenSubscriberCount,
			VideoCount:       parseCount(it.Statistics.VideoCount),
		})
	}
	return out
}

// Channels returns public information for up to 50 channels. Cost: 1 unit.
func (c *DataClient) Channels(ctx context.Context, ids []string) ([]ChannelInfo, error) {
	if c.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 50 {
		return nil, errors.New("at most 50 channel IDs per request")
	}
	var resp channelsResponse
	q := url.Values{"part": {"snippet,statistics"}, "id": {strings.Join(ids, ",")}, "maxResults": {"50"}}
	if err := c.get(ctx, "/channels", q, CostChannels, "", &resp); err != nil {
		return nil, err
	}
	return resp.infos(), nil
}

// ChannelByHandle looks up a channel by handle (without "@"). It returns
// (nil, nil) when no channel uses the handle. Cost: 1 unit.
func (c *DataClient) ChannelByHandle(ctx context.Context, handle string) (*ChannelInfo, error) {
	if c.apiKey == "" {
		return nil, ErrNotConfigured
	}
	var resp channelsResponse
	q := url.Values{"part": {"snippet,statistics"}, "forHandle": {"@" + strings.TrimPrefix(handle, "@")}}
	if err := c.get(ctx, "/channels", q, CostChannels, "", &resp); err != nil {
		return nil, err
	}
	infos := resp.infos()
	if len(infos) == 0 {
		return nil, nil
	}
	return &infos[0], nil
}

// MyChannels lists the channels owned by the authorized user. Cost: 1 unit.
func (c *DataClient) MyChannels(ctx context.Context, accessToken string) ([]ChannelInfo, error) {
	if accessToken == "" {
		return nil, ErrUnauthorized
	}
	var resp channelsResponse
	q := url.Values{"part": {"snippet,statistics"}, "mine": {"true"}, "maxResults": {"50"}}
	if err := c.get(ctx, "/channels", q, CostChannels, accessToken, &resp); err != nil {
		return nil, err
	}
	return resp.infos(), nil
}

const maxBody = 4 << 20

// get performs a GET after reserving quota. Requests authenticated with an
// access token do not send the API key. URLs are never logged because they
// can contain the API key.
func (c *DataClient) get(ctx context.Context, path string, q url.Values, cost int, accessToken string, out any) error {
	if c.quota != nil {
		if err := c.quota.Reserve(ctx, cost); err != nil {
			return err
		}
	}
	if accessToken == "" {
		q.Set("key", c.apiKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Strip the URL from transport errors; it may contain the key.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: read response: %v", ErrUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return classifyError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrUnavailable, err)
	}
	return nil
}

func classifyError(status int, body []byte) error {
	var g struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &g)
	apiErr := &APIError{Status: status, Message: g.Error.Message}
	if len(g.Error.Errors) > 0 {
		apiErr.Reason = g.Error.Errors[0].Reason
	}
	switch {
	case apiErr.Reason == "quotaExceeded" || apiErr.Reason == "dailyLimitExceeded" || apiErr.Reason == "rateLimitExceeded":
		return fmt.Errorf("%w: %v", ErrQuotaExceeded, apiErr)
	case status == http.StatusUnauthorized || apiErr.Reason == "keyInvalid" || apiErr.Reason == "authError":
		return fmt.Errorf("%w: %v", ErrUnauthorized, apiErr)
	case status >= 500:
		return fmt.Errorf("%w: %v", ErrUnavailable, apiErr)
	default:
		return apiErr
	}
}

func parseCount(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
