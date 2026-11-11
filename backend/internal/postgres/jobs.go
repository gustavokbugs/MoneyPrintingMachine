package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/jobs"
)

// JobStore implements jobs.Store.
type JobStore struct {
	pool *pgxpool.Pool
}

// NewJobStore builds a JobStore.
func NewJobStore(pool *pgxpool.Pool) *JobStore { return &JobStore{pool: pool} }

const jobColumns = `id::text, owner_id, kind, coalesce(channel_id::text, ''), status, input, result,
	error, attempts, max_attempts, created_at, updated_at`

func scanJob(row pgx.Row) (jobs.Job, error) {
	var (
		j      jobs.Job
		status string
		result []byte
	)
	err := row.Scan(&j.ID, &j.OwnerID, &j.Kind, &j.ChannelID, &status, &j.Input, &result,
		&j.Error, &j.Attempts, &j.MaxAttempts, &j.CreatedAt, &j.UpdatedAt)
	j.Status = jobs.Status(status)
	if len(result) > 0 {
		j.Result = json.RawMessage(result)
	}
	return j, err
}

// Enqueue inserts a job, or returns the existing job with the same
// idempotency key for this owner.
func (s *JobStore) Enqueue(ctx context.Context, nj jobs.NewJob, input json.RawMessage) (jobs.Job, error) {
	var channelID, idemKey any
	if nj.ChannelID != "" {
		channelID = nj.ChannelID
	}
	if nj.IdempotencyKey != "" {
		idemKey = nj.IdempotencyKey
	}
	j, err := scanJob(s.pool.QueryRow(ctx, `INSERT INTO jobs (owner_id, kind, channel_id, status, input, max_attempts, idempotency_key)
		VALUES ($1, $2, $3::uuid, 'queued', $4, $5, $6)
		ON CONFLICT (owner_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
		RETURNING `+jobColumns, nj.OwnerID, nj.Kind, channelID, input, nj.MaxAttempts, idemKey))
	if isNoRows(err) && nj.IdempotencyKey != "" {
		return scanJob(s.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs
			WHERE owner_id = $1 AND idempotency_key = $2`, nj.OwnerID, nj.IdempotencyKey))
	}
	if err != nil {
		return jobs.Job{}, fmt.Errorf("insert job: %w", err)
	}
	return j, nil
}

// Get returns a job owned by ownerID.
func (s *JobStore) Get(ctx context.Context, ownerID, id string) (jobs.Job, error) {
	j, err := scanJob(s.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = $1 AND owner_id = $2`, id, ownerID))
	if isNoRows(err) || isInvalidUUID(err) {
		return jobs.Job{}, jobs.ErrNotFound
	}
	if err != nil {
		return jobs.Job{}, fmt.Errorf("get job: %w", err)
	}
	return j, nil
}

// ListRecent returns the owner's jobs updated since the given time, newest
// first, at most limit, leaving out the excluded kinds (background
// housekeeping the user did not ask for).
func (s *JobStore) ListRecent(ctx context.Context, ownerID string, since time.Time, limit int, exclude []string) ([]jobs.Job, error) {
	if exclude == nil {
		exclude = []string{}
	}
	rows, err := s.pool.Query(ctx, `SELECT `+jobColumns+` FROM jobs
		WHERE owner_id = $1 AND updated_at >= $2 AND NOT (kind = ANY($3))
		ORDER BY updated_at DESC LIMIT $4`, ownerID, since, exclude, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	out := []jobs.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Claim leases the oldest runnable job. Jobs whose lease expired (for
// example after a crash) become claimable again.
func (s *JobStore) Claim(ctx context.Context, kinds []string, lease time.Duration) (*jobs.Job, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	j, err := scanJob(s.pool.QueryRow(ctx, `UPDATE jobs SET
			status = 'running', attempts = attempts + 1,
			locked_until = now() + make_interval(secs => $2), updated_at = now()
		WHERE id = (
			SELECT id FROM jobs
			WHERE kind = ANY($1) AND (
				(status = 'queued' AND run_after <= now()) OR
				(status = 'running' AND locked_until < now()))
			ORDER BY run_after
			FOR UPDATE SKIP LOCKED
			LIMIT 1)
		RETURNING `+jobColumns, kinds, lease.Seconds()))
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}
	return &j, nil
}

// Complete marks a job as succeeded with its result.
func (s *JobStore) Complete(ctx context.Context, id string, result json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status = 'succeeded', result = $2, error = '',
		locked_until = NULL, updated_at = now() WHERE id = $1`, id, result)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

// Fail records a failure and either reschedules or fails permanently.
func (s *JobStore) Fail(ctx context.Context, id string, msg string, retryAt *time.Time) error {
	var err error
	if retryAt != nil {
		_, err = s.pool.Exec(ctx, `UPDATE jobs SET status = 'queued', error = $2, run_after = $3,
			locked_until = NULL, updated_at = now() WHERE id = $1`, id, msg, *retryAt)
	} else {
		_, err = s.pool.Exec(ctx, `UPDATE jobs SET status = 'failed', error = $2,
			locked_until = NULL, updated_at = now() WHERE id = $1`, id, msg)
	}
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	return nil
}

// PruneFinished deletes succeeded and failed jobs last updated before the
// cutoff. Queued and running jobs are never removed.
func (s *JobStore) PruneFinished(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM jobs WHERE status IN ('succeeded', 'failed') AND updated_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("prune jobs: %w", err)
	}
	return tag.RowsAffected(), nil
}
