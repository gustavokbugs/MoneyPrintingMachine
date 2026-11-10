package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/trends"
)

// TrendSnapshotRepository implements trends.Store.
type TrendSnapshotRepository struct {
	pool *pgxpool.Pool
}

// NewTrendSnapshotRepository builds a TrendSnapshotRepository.
func NewTrendSnapshotRepository(pool *pgxpool.Pool) *TrendSnapshotRepository {
	return &TrendSnapshotRepository{pool: pool}
}

const snapshotColumns = `id, source, query, query_key, region, language, metrics, collected_at`

func scanSnapshot(row pgx.Row) (trends.Snapshot, error) {
	var (
		s   trends.Snapshot
		raw []byte
	)
	if err := row.Scan(&s.ID, &s.Source, &s.Query, &s.QueryKey, &s.Region, &s.Language, &raw, &s.CollectedAt); err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s.Metrics); err != nil {
		return s, fmt.Errorf("decode metrics: %w", err)
	}
	return s, nil
}

// Latest returns the newest snapshot collected at or before `before`.
func (r *TrendSnapshotRepository) Latest(ctx context.Context, ownerID, source, queryKey, region, language string, before time.Time) (*trends.Snapshot, error) {
	s, err := scanSnapshot(r.pool.QueryRow(ctx, `SELECT `+snapshotColumns+` FROM trend_snapshots
		WHERE owner_id = $1 AND source = $2 AND query_key = $3 AND region = $4 AND language = $5 AND collected_at <= $6
		ORDER BY collected_at DESC LIMIT 1`, ownerID, source, queryKey, region, language, before))
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest snapshot: %w", err)
	}
	return &s, nil
}

// Save inserts a snapshot and sets its ID.
func (r *TrendSnapshotRepository) Save(ctx context.Context, ownerID string, s *trends.Snapshot) error {
	raw, err := json.Marshal(s.Metrics)
	if err != nil {
		return fmt.Errorf("encode metrics: %w", err)
	}
	err = r.pool.QueryRow(ctx, `INSERT INTO trend_snapshots (owner_id, source, query_key, query, region, language, metrics, collected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		ownerID, s.Source, s.QueryKey, s.Query, s.Region, s.Language, raw, s.CollectedAt).Scan(&s.ID)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	return nil
}

// History returns snapshots newest first, at most limit.
func (r *TrendSnapshotRepository) History(ctx context.Context, ownerID, source, queryKey, region, language string, limit int) ([]trends.Snapshot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+snapshotColumns+` FROM trend_snapshots
		WHERE owner_id = $1 AND source = $2 AND query_key = $3 AND region = $4 AND language = $5
		ORDER BY collected_at DESC LIMIT $6`, ownerID, source, queryKey, region, language, limit)
	if err != nil {
		return nil, fmt.Errorf("snapshot history: %w", err)
	}
	defer rows.Close()
	out := []trends.Snapshot{}
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
