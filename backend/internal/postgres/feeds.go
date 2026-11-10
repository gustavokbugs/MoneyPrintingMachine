package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/rss"
)

// FeedRepository implements rss.Store.
type FeedRepository struct {
	pool *pgxpool.Pool
}

// NewFeedRepository builds a FeedRepository.
func NewFeedRepository(pool *pgxpool.Pool) *FeedRepository { return &FeedRepository{pool: pool} }

const feedColumns = `f.id, f.owner_id, f.url, f.title, f.active, f.etag, f.last_modified, f.next_fetch_at,
	f.last_fetched_at, f.last_status, f.last_error, f.created_at`

func scanFeed(row pgx.Row, withCount bool) (rss.FeedRecord, error) {
	var f rss.FeedRecord
	dest := []any{&f.ID, &f.OwnerID, &f.URL, &f.Title, &f.Active, &f.ETag, &f.LastModified, &f.NextFetchAt,
		&f.LastFetchedAt, &f.LastStatus, &f.LastError, &f.CreatedAt}
	if withCount {
		dest = append(dest, &f.ItemCount)
	}
	return f, row.Scan(dest...)
}

// Add stores a new feed, due immediately.
func (r *FeedRepository) Add(ctx context.Context, ownerID, url string) (rss.FeedRecord, error) {
	f, err := scanFeed(r.pool.QueryRow(ctx, `INSERT INTO rss_feeds AS f (owner_id, url) VALUES ($1, $2)
		RETURNING `+feedColumns, ownerID, url), false)
	if isUniqueViolation(err) {
		return rss.FeedRecord{}, rss.ErrFeedExists
	}
	if err != nil {
		return rss.FeedRecord{}, fmt.Errorf("insert feed: %w", err)
	}
	return f, nil
}

// Count returns how many feeds the owner has.
func (r *FeedRepository) Count(ctx context.Context, ownerID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM rss_feeds WHERE owner_id = $1`, ownerID).Scan(&n)
	return n, err
}

// CountActiveFeeds counts active feeds that fetched successfully at least once.
func (r *FeedRepository) CountActiveFeeds(ctx context.Context, ownerID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM rss_feeds WHERE owner_id = $1 AND active AND last_fetched_at IS NOT NULL`, ownerID).Scan(&n)
	return n, err
}

// List returns feeds with their stored item counts in one query.
func (r *FeedRepository) List(ctx context.Context, ownerID string) ([]rss.FeedRecord, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+feedColumns+`, coalesce(c.n, 0)
		FROM rss_feeds f
		LEFT JOIN (SELECT feed_id, count(*) AS n FROM rss_items WHERE owner_id = $1 GROUP BY feed_id) c ON c.feed_id = f.id
		WHERE f.owner_id = $1 ORDER BY f.created_at`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list feeds: %w", err)
	}
	defer rows.Close()
	out := []rss.FeedRecord{}
	for rows.Next() {
		f, err := scanFeed(rows, true)
		if err != nil {
			return nil, fmt.Errorf("scan feed: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Delete removes a feed and, by cascade, its items.
func (r *FeedRepository) Delete(ctx context.Context, ownerID string, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM rss_feeds WHERE id = $1 AND owner_id = $2`, id, ownerID)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return rss.ErrFeedNotFound
	}
	return nil
}

// ClaimDue leases due feeds.
func (r *FeedRepository) ClaimDue(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]rss.FeedRecord, error) {
	rows, err := r.pool.Query(ctx, `UPDATE rss_feeds AS f SET next_fetch_at = $1::timestamptz + make_interval(secs => $3)
		WHERE f.id IN (
			SELECT id FROM rss_feeds WHERE active AND next_fetch_at <= $1
			ORDER BY next_fetch_at FOR UPDATE SKIP LOCKED LIMIT $2)
		RETURNING `+feedColumns, now, limit, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim feeds: %w", err)
	}
	defer rows.Close()
	var out []rss.FeedRecord
	for rows.Next() {
		f, err := scanFeed(rows, false)
		if err != nil {
			return nil, fmt.Errorf("scan feed: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SaveOutcome updates fetch state and inserts new items atomically.
func (r *FeedRepository) SaveOutcome(ctx context.Context, f rss.FeedRecord, o rss.FetchOutcome) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save outcome: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE rss_feeds SET title = $2, etag = $3, last_modified = $4, next_fetch_at = $5,
			last_fetched_at = CASE WHEN $6 = 'error' THEN last_fetched_at ELSE $7 END,
			last_status = $6, last_error = $8
		WHERE id = $1`, f.ID, o.Title, o.ETag, o.LastModified, o.NextFetchAt, o.Status, o.FetchedAt, o.Error); err != nil {
		return fmt.Errorf("update feed: %w", err)
	}
	if len(o.Items) > 0 {
		hashes := make([]string, len(o.Items))
		titles := make([]string, len(o.Items))
		links := make([]string, len(o.Items))
		dates := make([]time.Time, len(o.Items))
		for i, it := range o.Items {
			hashes[i], titles[i], links[i], dates[i] = it.GUIDHash, it.Title, it.Link, it.PublishedAt
		}
		if _, err := tx.Exec(ctx, `INSERT INTO rss_items (feed_id, owner_id, guid_hash, title, link, published_at)
			SELECT $1, $2, h, t, l, d FROM unnest($3::text[], $4::text[], $5::text[], $6::timestamptz[]) AS x(h, t, l, d)
			ON CONFLICT (feed_id, guid_hash) DO NOTHING`, f.ID, f.OwnerID, hashes, titles, links, dates); err != nil {
			return fmt.Errorf("insert items: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// CountMentions counts items whose titles contain every token. Tokens are
// lowercase letters and digits only (see rss.Tokens), so they never contain
// LIKE wildcards.
func (r *FeedRepository) CountMentions(ctx context.Context, ownerID string, tokens []string, from, to time.Time) (int, error) {
	patterns := make([]string, len(tokens))
	for i, t := range tokens {
		patterns[i] = "%" + t + "%"
	}
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM rss_items
		WHERE owner_id = $1 AND published_at >= $2 AND published_at < $3 AND title ILIKE ALL($4::text[])`,
		ownerID, from, to, patterns).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count mentions: %w", err)
	}
	return n, nil
}

// PruneItems deletes items published before the cutoff.
func (r *FeedRepository) PruneItems(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM rss_items WHERE published_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("prune items: %w", err)
	}
	return tag.RowsAffected(), nil
}
