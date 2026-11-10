// Package trends measures niche demand and competition from official data
// sources and keeps point-in-time snapshots so momentum can be computed.
//
// Every score is derived from measured values with the documented formulas
// below. When a value cannot be measured it is left nil; it is never
// estimated or filled in.
package trends

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"contentfactory/internal/youtube"
)

// SourceYouTube identifies measurements from the YouTube Data API.
const SourceYouTube = "youtube_data_api"

// WindowDays is the lookback window for recent videos.
const WindowDays = 30

// Scoring reference points. A median of ReferenceViewsPerDay views per day
// scores 100 for demand; a median of ReferenceSubscribers subscribers
// scores 100 for competition. Both scales are logarithmic.
const (
	ReferenceViewsPerDay = 50_000
	ReferenceSubscribers = 10_000_000
)

// ErrInvalidQuery is returned for an empty or oversized query.
var ErrInvalidQuery = errors.New("query must be between 1 and 200 characters")

// Metrics are raw measurements for one query at one point in time.
type Metrics struct {
	SampleSize           int     `json:"sampleSize"`
	WindowDays           int     `json:"windowDays"`
	MedianViews          int64   `json:"medianViews"`
	MedianViewsPerDay    float64 `json:"medianViewsPerDay"`
	MedianEngagementRate float64 `json:"medianEngagementRate"`
	DistinctChannels     int     `json:"distinctChannels"`
	TopChannelShare      float64 `json:"topChannelShare"`
	// MedianSubscribers is computed over channels with public counts;
	// nil when none are public.
	MedianSubscribers        *int64 `json:"medianSubscribers"`
	HiddenSubscriberChannels int    `json:"hiddenSubscriberChannels"`
}

// Scores are 0-100 values derived from Metrics. Nil means not computable.
type Scores struct {
	// Demand: 100 * log10(1 + medianViewsPerDay) / log10(1 + 50,000).
	Demand *float64 `json:"demand"`
	// Competition: 100 * log10(1 + medianSubscribers) / log10(1 + 10,000,000).
	// Higher means established channels dominate the top results.
	Competition *float64 `json:"competition"`
	// MomentumPercent: change in median views per day versus the previous
	// snapshot that is at least MinMomentumGap older.
	MomentumPercent *float64 `json:"momentumPercent"`
	// Trend: 50 + clamp(momentumPercent, -100, 100) / 2.
	Trend *float64 `json:"trend"`
}

// Snapshot is one stored measurement.
type Snapshot struct {
	ID          int64     `json:"id"`
	Source      string    `json:"source"`
	Query       string    `json:"query"`
	QueryKey    string    `json:"queryKey"`
	Region      string    `json:"region"`
	Language    string    `json:"language"`
	Metrics     Metrics   `json:"metrics"`
	CollectedAt time.Time `json:"collectedAt"`
}

// Result is a measurement with its derived scores.
type Result struct {
	Snapshot Snapshot  `json:"snapshot"`
	Scores   Scores    `json:"scores"`
	Previous *Snapshot `json:"previous"`
	// Cached is true when a recent snapshot was reused instead of calling
	// the API again.
	Cached bool     `json:"cached"`
	Notes  []string `json:"notes"`
}

// YouTube is the subset of the YouTube Data API used for measurements.
type YouTube interface {
	HasAPIKey() bool
	SearchVideoIDs(ctx context.Context, p youtube.SearchParams) ([]string, error)
	Videos(ctx context.Context, ids []string) ([]youtube.Video, error)
	Channels(ctx context.Context, ids []string) ([]youtube.ChannelInfo, error)
}

// Store persists snapshots.
type Store interface {
	// Latest returns the newest snapshot collected at or before `before`,
	// or nil when there is none.
	Latest(ctx context.Context, ownerID, source, queryKey, region, language string, before time.Time) (*Snapshot, error)
	Save(ctx context.Context, ownerID string, s *Snapshot) error
	History(ctx context.Context, ownerID, source, queryKey, region, language string, limit int) ([]Snapshot, error)
}

// Service measures queries and stores snapshots.
type Service struct {
	yt    YouTube
	store Store
	now   func() time.Time
	// CacheTTL is how long a snapshot is reused before measuring again.
	CacheTTL time.Duration
	// MinMomentumGap is the minimum age difference for momentum.
	MinMomentumGap time.Duration
}

// NewService builds a Service.
func NewService(yt YouTube, store Store) *Service {
	return &Service{yt: yt, store: store, now: time.Now, CacheTTL: 6 * time.Hour, MinMomentumGap: 12 * time.Hour}
}

// Available reports whether measurements can be taken.
func (s *Service) Available() bool { return s.yt != nil && s.yt.HasAPIKey() }

// Request identifies what to measure.
type Request struct {
	OwnerID  string
	Query    string
	Region   string // ISO 3166-1 alpha-2, e.g. "US".
	Language string // BCP 47 tag, e.g. "en-US".
}

// QueryKey normalizes a query for snapshot lookups.
func QueryKey(q string) string { return strings.ToLower(strings.Join(strings.Fields(q), " ")) }

func baseLanguage(tag string) string {
	base, _, _ := strings.Cut(tag, "-")
	return strings.ToLower(base)
}

// Measure returns fresh or recently cached measurements for a query.
func (s *Service) Measure(ctx context.Context, r Request) (Result, error) {
	query := strings.Join(strings.Fields(r.Query), " ")
	if query == "" || len(query) > 200 {
		return Result{}, ErrInvalidQuery
	}
	if !s.Available() {
		return Result{}, youtube.ErrNotConfigured
	}
	key := QueryKey(query)
	region := strings.ToUpper(r.Region)
	now := s.now().UTC()

	latest, err := s.store.Latest(ctx, r.OwnerID, SourceYouTube, key, region, r.Language, now)
	if err != nil {
		return Result{}, fmt.Errorf("load latest snapshot: %w", err)
	}
	if latest != nil && now.Sub(latest.CollectedAt) < s.CacheTTL {
		return s.result(ctx, r.OwnerID, *latest, true)
	}

	m, err := s.collect(ctx, query, region, baseLanguage(r.Language), now)
	if err != nil {
		return Result{}, err
	}
	snap := Snapshot{
		Source: SourceYouTube, Query: query, QueryKey: key, Region: region,
		Language: r.Language, Metrics: m, CollectedAt: now,
	}
	if err := s.store.Save(ctx, r.OwnerID, &snap); err != nil {
		return Result{}, fmt.Errorf("save snapshot: %w", err)
	}
	return s.result(ctx, r.OwnerID, snap, false)
}

func (s *Service) result(ctx context.Context, owner string, snap Snapshot, cached bool) (Result, error) {
	prev, err := s.store.Latest(ctx, owner, snap.Source, snap.QueryKey, snap.Region, snap.Language,
		snap.CollectedAt.Add(-s.MinMomentumGap))
	if err != nil {
		return Result{}, fmt.Errorf("load previous snapshot: %w", err)
	}
	var prevMetrics *Metrics
	if prev != nil {
		prevMetrics = &prev.Metrics
	}
	res := Result{Snapshot: snap, Scores: ComputeScores(snap.Metrics, prevMetrics), Previous: prev, Cached: cached, Notes: []string{}}
	if snap.Metrics.SampleSize == 0 {
		res.Notes = append(res.Notes, "No videos matching this query were published in the last 30 days.")
	}
	if prev == nil {
		res.Notes = append(res.Notes, "Momentum needs an earlier snapshot at least 12 hours old; it will appear after the next measurement.")
	}
	return res, nil
}

// collect runs the API calls: search (100 units), videos (1), channels (1).
func (s *Service) collect(ctx context.Context, query, region, lang string, now time.Time) (Metrics, error) {
	ids, err := s.yt.SearchVideoIDs(ctx, youtube.SearchParams{
		Query: query, RegionCode: region, Language: lang,
		PublishedAfter: now.AddDate(0, 0, -WindowDays), MaxResults: 50,
	})
	if err != nil {
		return Metrics{}, fmt.Errorf("search videos: %w", err)
	}
	videos, err := s.yt.Videos(ctx, ids)
	if err != nil {
		return Metrics{}, fmt.Errorf("load video statistics: %w", err)
	}
	channelIDs := distinctChannels(videos)
	if len(channelIDs) > 50 {
		channelIDs = channelIDs[:50]
	}
	channels, err := s.yt.Channels(ctx, channelIDs)
	if err != nil {
		return Metrics{}, fmt.Errorf("load channel statistics: %w", err)
	}
	return ComputeMetrics(videos, channels, now), nil
}

func distinctChannels(videos []youtube.Video) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range videos {
		if v.ChannelID != "" && !seen[v.ChannelID] {
			seen[v.ChannelID] = true
			out = append(out, v.ChannelID)
		}
	}
	return out
}

// ComputeMetrics derives metrics from raw API data.
func ComputeMetrics(videos []youtube.Video, channels []youtube.ChannelInfo, now time.Time) Metrics {
	m := Metrics{SampleSize: len(videos), WindowDays: WindowDays}
	if len(videos) == 0 {
		return m
	}
	views := make([]float64, 0, len(videos))
	perDay := make([]float64, 0, len(videos))
	engagement := make([]float64, 0, len(videos))
	perChannel := map[string]int{}
	for _, v := range videos {
		views = append(views, float64(v.ViewCount))
		ageDays := now.Sub(v.PublishedAt).Hours() / 24
		if ageDays < 1 {
			ageDays = 1
		}
		perDay = append(perDay, float64(v.ViewCount)/ageDays)
		if v.ViewCount > 0 {
			engagement = append(engagement, float64(v.LikeCount+v.CommentCount)/float64(v.ViewCount))
		}
		perChannel[v.ChannelID]++
	}
	m.MedianViews = int64(median(views))
	m.MedianViewsPerDay = round2(median(perDay))
	m.MedianEngagementRate = round4(median(engagement))
	m.DistinctChannels = len(perChannel)
	top := 0
	for _, n := range perChannel {
		top = max(top, n)
	}
	m.TopChannelShare = round4(float64(top) / float64(len(videos)))

	subs := make([]float64, 0, len(channels))
	for _, c := range channels {
		if c.HiddenSubscriber {
			m.HiddenSubscriberChannels++
			continue
		}
		subs = append(subs, float64(c.SubscriberCount))
	}
	if len(subs) > 0 {
		v := int64(median(subs))
		m.MedianSubscribers = &v
	}
	return m
}

// ComputeScores derives 0-100 scores; prev may be nil.
func ComputeScores(m Metrics, prev *Metrics) Scores {
	var s Scores
	demand := logScore(m.MedianViewsPerDay, ReferenceViewsPerDay)
	s.Demand = &demand
	if m.MedianSubscribers != nil {
		c := logScore(float64(*m.MedianSubscribers), ReferenceSubscribers)
		s.Competition = &c
	}
	if prev != nil && prev.MedianViewsPerDay > 0 && m.SampleSize > 0 {
		pct := round2((m.MedianViewsPerDay - prev.MedianViewsPerDay) / prev.MedianViewsPerDay * 100)
		trend := round2(50 + math.Max(-100, math.Min(100, pct))/2)
		s.MomentumPercent, s.Trend = &pct, &trend
	}
	return s
}

func logScore(v, reference float64) float64 {
	if v <= 0 {
		return 0
	}
	return round2(math.Min(100, 100*math.Log10(1+v)/math.Log10(1+reference)))
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// History returns recent snapshots for a query, newest first.
func (s *Service) History(ctx context.Context, ownerID, query, region, language string, limit int) ([]Snapshot, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.store.History(ctx, ownerID, SourceYouTube, QueryKey(query), strings.ToUpper(region), language, limit)
}
