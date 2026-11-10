package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/trends"
)

// WatchlistRepository implements trends.WatchStore.
type WatchlistRepository struct {
	pool *pgxpool.Pool
}

// NewWatchlistRepository builds a WatchlistRepository.
func NewWatchlistRepository(pool *pgxpool.Pool) *WatchlistRepository {
	return &WatchlistRepository{pool: pool}
}

const watchColumns = `id, owner_id, query, query_key, region, language, source, active, next_due_at, last_measured_at, last_error, created_at`

func scanWatch(row pgx.Row) (trends.Watch, error) {
	var w trends.Watch
	err := row.Scan(&w.ID, &w.OwnerID, &w.Query, &w.QueryKey, &w.Region, &w.Language, &w.Source, &w.Active,
		&w.NextDueAt, &w.LastMeasuredAt, &w.LastError, &w.CreatedAt)
	return w, err
}

func collectWatches(rows pgx.Rows) ([]trends.Watch, error) {
	defer rows.Close()
	out := []trends.Watch{}
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, fmt.Errorf("scan watch: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// normalizedNiche mirrors trends.QueryKey in SQL: trimmed, single-spaced,
// lowercase.
const normalizedNiche = `lower(regexp_replace(btrim(niche), '\s+', ' ', 'g'))`

// SyncChannelNiches keeps channel-sourced watches in step with channels.
func (r *WatchlistRepository) SyncChannelNiches(ctx context.Context) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin sync: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO trend_watchlist (owner_id, query_key, query, region, language, source)
		SELECT DISTINCT ON (owner_id, `+normalizedNiche+`, geo_target, language)
			owner_id, `+normalizedNiche+`, regexp_replace(btrim(niche), '\s+', ' ', 'g'), geo_target, language, 'channel'
		FROM channels WHERE status <> 'archived' AND btrim(niche) <> ''
		ON CONFLICT (owner_id, query_key, region, language) DO UPDATE SET active = true
		WHERE trend_watchlist.source = 'channel' AND NOT trend_watchlist.active`); err != nil {
		return fmt.Errorf("insert channel watches: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE trend_watchlist w SET active = false
		WHERE w.source = 'channel' AND w.active AND NOT EXISTS (
			SELECT 1 FROM channels c
			WHERE c.owner_id = w.owner_id AND c.status <> 'archived'
			  AND lower(regexp_replace(btrim(c.niche), '\s+', ' ', 'g')) = w.query_key
			  AND c.geo_target = w.region AND c.language = w.language)`); err != nil {
		return fmt.Errorf("deactivate stale watches: %w", err)
	}
	return tx.Commit(ctx)
}

// ClaimDue leases due watches.
func (r *WatchlistRepository) ClaimDue(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]trends.Watch, error) {
	rows, err := r.pool.Query(ctx, `UPDATE trend_watchlist SET next_due_at = $1::timestamptz + make_interval(secs => $3)
		WHERE id IN (
			SELECT id FROM trend_watchlist WHERE active AND next_due_at <= $1
			ORDER BY next_due_at FOR UPDATE SKIP LOCKED LIMIT $2)
		RETURNING `+watchColumns, now, limit, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim watches: %w", err)
	}
	return collectWatches(rows)
}

// MarkMeasured records a successful measurement.
func (r *WatchlistRepository) MarkMeasured(ctx context.Context, id int64, at, next time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE trend_watchlist SET last_measured_at = $2, next_due_at = $3, last_error = '' WHERE id = $1`, id, at, next)
	if err != nil {
		return fmt.Errorf("mark measured: %w", err)
	}
	return nil
}

// MarkFailed records a failure and the retry time.
func (r *WatchlistRepository) MarkFailed(ctx context.Context, id int64, msg string, retryAt time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE trend_watchlist SET last_error = $2, next_due_at = $3 WHERE id = $1`, id, msg, retryAt)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

// List returns active watches first, then by query.
func (r *WatchlistRepository) List(ctx context.Context, ownerID string, limit int) ([]trends.Watch, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+watchColumns+` FROM trend_watchlist WHERE owner_id = $1
		ORDER BY active DESC, query_key LIMIT $2`, ownerID, limit)
	if err != nil {
		return nil, fmt.Errorf("list watches: %w", err)
	}
	return collectWatches(rows)
}

// AddManual adds a watch, or reactivates an existing one for the same key.
func (r *WatchlistRepository) AddManual(ctx context.Context, w trends.Watch) (trends.Watch, error) {
	out, err := scanWatch(r.pool.QueryRow(ctx, `INSERT INTO trend_watchlist (owner_id, query_key, query, region, language, source)
		VALUES ($1, $2, $3, $4, $5, 'manual')
		ON CONFLICT (owner_id, query_key, region, language) DO UPDATE SET active = true
		RETURNING `+watchColumns, w.OwnerID, w.QueryKey, w.Query, w.Region, w.Language))
	if err != nil {
		return trends.Watch{}, fmt.Errorf("add watch: %w", err)
	}
	return out, nil
}

// Delete removes a watch. Channel watches come back on the next sync while
// the channel exists, so only manual watches are deleted.
func (r *WatchlistRepository) Delete(ctx context.Context, ownerID string, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM trend_watchlist WHERE id = $1 AND owner_id = $2 AND source = 'manual'`, id, ownerID)
	if err != nil {
		return fmt.Errorf("delete watch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return trends.ErrWatchNotFound
	}
	return nil
}
