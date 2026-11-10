package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// QuotaStore records external API quota usage per provider and day.
type QuotaStore struct {
	pool *pgxpool.Pool
}

// NewQuotaStore builds a QuotaStore.
func NewQuotaStore(pool *pgxpool.Pool) *QuotaStore { return &QuotaStore{pool: pool} }

// Reserve atomically adds units if the day's total stays within limit.
func (s *QuotaStore) Reserve(ctx context.Context, provider string, day time.Time, units, limit int) (bool, error) {
	if units > limit {
		return false, nil
	}
	var total int
	err := s.pool.QueryRow(ctx, `INSERT INTO api_quota_usage (provider, usage_day, units) VALUES ($1, $2, $3)
		ON CONFLICT (provider, usage_day) DO UPDATE SET units = api_quota_usage.units + EXCLUDED.units
		WHERE api_quota_usage.units + EXCLUDED.units <= $4
		RETURNING units`, provider, day, units, limit).Scan(&total)
	if isNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reserve quota: %w", err)
	}
	return true, nil
}

// Used returns the units consumed on a day.
func (s *QuotaStore) Used(ctx context.Context, provider string, day time.Time) (int, error) {
	var used int
	err := s.pool.QueryRow(ctx, `SELECT coalesce((SELECT units FROM api_quota_usage WHERE provider = $1 AND usage_day = $2), 0)`,
		provider, day).Scan(&used)
	if err != nil {
		return 0, fmt.Errorf("read quota usage: %w", err)
	}
	return used, nil
}
