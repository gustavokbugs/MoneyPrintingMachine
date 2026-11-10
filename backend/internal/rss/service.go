package rss

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrFeedNotFound is returned when a feed does not exist for the owner.
	ErrFeedNotFound = errors.New("feed not found")
	// ErrFeedExists is returned when the owner already added the URL.
	ErrFeedExists = errors.New("this feed is already added")
	// ErrTooManyFeeds bounds the number of feeds per owner.
	ErrTooManyFeeds = errors.New("feed limit reached (50 feeds)")
)

// MaxFeedsPerOwner bounds polling work per owner.
const MaxFeedsPerOwner = 50

// FeedRecord is a stored feed and its fetch state.
type FeedRecord struct {
	ID            int64      `json:"id"`
	OwnerID       string     `json:"-"`
	URL           string     `json:"url"`
	Title         string     `json:"title"`
	Active        bool       `json:"active"`
	ETag          string     `json:"-"`
	LastModified  string     `json:"-"`
	NextFetchAt   time.Time  `json:"nextFetchAt"`
	LastFetchedAt *time.Time `json:"lastFetchedAt"`
	LastStatus    string     `json:"lastStatus"`
	LastError     string     `json:"lastError"`
	ItemCount     int        `json:"itemCount"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// FetchOutcome is persisted after a fetch attempt.
type FetchOutcome struct {
	Status       string // "ok", "not_modified", or "error".
	Error        string
	Title        string
	ETag         string
	LastModified string
	Items        []Item
	FetchedAt    time.Time
	NextFetchAt  time.Time
}

// Store persists feeds and items.
type Store interface {
	Add(ctx context.Context, ownerID, url string) (FeedRecord, error)
	Count(ctx context.Context, ownerID string) (int, error)
	List(ctx context.Context, ownerID string) ([]FeedRecord, error)
	Delete(ctx context.Context, ownerID string, id int64) error
	// ClaimDue leases due feeds by pushing their next fetch time forward.
	ClaimDue(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]FeedRecord, error)
	SaveOutcome(ctx context.Context, feed FeedRecord, o FetchOutcome) error
	// CountMentions counts items whose titles contain every token, in
	// [from, to).
	CountMentions(ctx context.Context, ownerID string, tokens []string, from, to time.Time) (int, error)
	CountActiveFeeds(ctx context.Context, ownerID string) (int, error)
	PruneItems(ctx context.Context, before time.Time) (int64, error)
}

// FeedFetcher downloads a feed.
type FeedFetcher interface {
	Fetch(ctx context.Context, url, etag, lastModified string) (FetchResult, error)
}

// Service manages feeds and computes news mentions.
type Service struct {
	store   Store
	fetcher FeedFetcher
	now     func() time.Time
	// PollInterval is how often each feed is fetched.
	PollInterval time.Duration
	// Retention is how long items are kept.
	Retention time.Duration
}

// NewService builds a Service polling every 30 minutes and keeping items
// for 60 days.
func NewService(store Store, fetcher FeedFetcher) *Service {
	return &Service{store: store, fetcher: fetcher, now: time.Now, PollInterval: 30 * time.Minute, Retention: 60 * 24 * time.Hour}
}

// AddFeed validates and stores a feed URL. The first fetch happens on the
// next scheduler run.
func (s *Service) AddFeed(ctx context.Context, ownerID, rawURL string) (FeedRecord, error) {
	u, err := ValidateURL(rawURL)
	if err != nil {
		return FeedRecord{}, err
	}
	n, err := s.store.Count(ctx, ownerID)
	if err != nil {
		return FeedRecord{}, fmt.Errorf("count feeds: %w", err)
	}
	if n >= MaxFeedsPerOwner {
		return FeedRecord{}, ErrTooManyFeeds
	}
	return s.store.Add(ctx, ownerID, u.String())
}

// List returns the owner's feeds.
func (s *Service) List(ctx context.Context, ownerID string) ([]FeedRecord, error) {
	return s.store.List(ctx, ownerID)
}

// Delete removes a feed and its items.
func (s *Service) Delete(ctx context.Context, ownerID string, id int64) error {
	return s.store.Delete(ctx, ownerID, id)
}

// FetchReport summarizes one polling run.
type FetchReport struct {
	Fetched int
	Failed  int
	Pruned  int64
}

// FetchDue polls up to limit due feeds and prunes old items.
func (s *Service) FetchDue(ctx context.Context, limit int) (FetchReport, error) {
	var rep FetchReport
	now := s.now().UTC()
	feeds, err := s.store.ClaimDue(ctx, now, limit, 10*time.Minute)
	if err != nil {
		return rep, fmt.Errorf("claim due feeds: %w", err)
	}
	for _, f := range feeds {
		if ctx.Err() != nil {
			return rep, ctx.Err()
		}
		fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		res, err := s.fetcher.Fetch(fctx, f.URL, f.ETag, f.LastModified)
		cancel()
		o := FetchOutcome{FetchedAt: now, NextFetchAt: now.Add(s.PollInterval), ETag: f.ETag, LastModified: f.LastModified, Title: f.Title}
		switch {
		case err != nil:
			rep.Failed++
			o.Status = "error"
			o.Error = truncate(err.Error(), 500)
			// Back off failing feeds to avoid hammering them.
			o.NextFetchAt = now.Add(4 * s.PollInterval)
		case res.NotModified:
			rep.Fetched++
			o.Status = "not_modified"
		default:
			rep.Fetched++
			o.Status = "ok"
			o.ETag, o.LastModified = res.ETag, res.LastModified
			if res.Feed.Title != "" {
				o.Title = truncate(res.Feed.Title, 200)
			}
			// Ignore entries dated in the future so they cannot inflate counts.
			for _, it := range res.Feed.Items {
				if !it.PublishedAt.After(now.Add(time.Hour)) {
					o.Items = append(o.Items, it)
				}
			}
		}
		if err := s.store.SaveOutcome(ctx, f, o); err != nil {
			return rep, fmt.Errorf("save feed outcome: %w", err)
		}
	}
	pruned, err := s.store.PruneItems(ctx, now.Add(-s.Retention))
	if err != nil {
		return rep, fmt.Errorf("prune items: %w", err)
	}
	rep.Pruned = pruned
	return rep, nil
}

// MentionStats compares how often a query appears in feed titles.
type MentionStats struct {
	Recent7d        int      `json:"recent7d"`
	Previous7d      int      `json:"previous7d"`
	MomentumPercent *float64 `json:"momentumPercent"`
	ActiveFeeds     int      `json:"activeFeeds"`
}

// Mentions counts titles containing every query token in the last 7 days
// and the 7 days before. Momentum is nil when the previous window is empty,
// because a percentage change from zero is undefined.
func (s *Service) Mentions(ctx context.Context, ownerID, query string) (MentionStats, error) {
	var st MentionStats
	active, err := s.store.CountActiveFeeds(ctx, ownerID)
	if err != nil {
		return st, fmt.Errorf("count feeds: %w", err)
	}
	st.ActiveFeeds = active
	tokens := Tokens(query)
	if active == 0 || len(tokens) == 0 {
		return st, nil
	}
	now := s.now().UTC()
	if st.Recent7d, err = s.store.CountMentions(ctx, ownerID, tokens, now.AddDate(0, 0, -7), now.Add(time.Hour)); err != nil {
		return st, fmt.Errorf("count recent mentions: %w", err)
	}
	if st.Previous7d, err = s.store.CountMentions(ctx, ownerID, tokens, now.AddDate(0, 0, -14), now.AddDate(0, 0, -7)); err != nil {
		return st, fmt.Errorf("count previous mentions: %w", err)
	}
	if st.Previous7d > 0 {
		pct := float64(st.Recent7d-st.Previous7d) / float64(st.Previous7d) * 100
		pct = float64(int(pct*100)) / 100
		st.MomentumPercent = &pct
	}
	return st, nil
}

// Available reports whether at least one feed is active.
func (s *Service) Available(ctx context.Context, ownerID string) bool {
	n, err := s.store.CountActiveFeeds(ctx, ownerID)
	return err == nil && n > 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
