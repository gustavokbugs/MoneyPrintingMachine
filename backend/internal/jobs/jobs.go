// Package jobs implements durable background jobs with leases, bounded
// retries, and bounded concurrency. Persistence lives behind Store.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

// Status is a job state.
type Status string

// Job states.
const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// ErrNotFound is returned when a job does not exist for the owner.
var ErrNotFound = errors.New("job not found")

// Job is a unit of durable background work.
type Job struct {
	ID          string          `json:"id"`
	OwnerID     string          `json:"-"`
	Kind        string          `json:"kind"`
	ChannelID   string          `json:"channelId,omitempty"`
	Status      Status          `json:"status"`
	Input       json.RawMessage `json:"input"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"maxAttempts"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// NewJob describes a job to enqueue.
type NewJob struct {
	OwnerID   string
	Kind      string
	ChannelID string
	Input     any
	// IdempotencyKey deduplicates enqueue requests per owner. When a job
	// with the same key exists, it is returned instead of creating another.
	IdempotencyKey string
	MaxAttempts    int
}

// Store persists jobs.
type Store interface {
	Enqueue(ctx context.Context, j NewJob, input json.RawMessage) (Job, error)
	Get(ctx context.Context, ownerID, id string) (Job, error)
	// Claim leases the next runnable job of one of the given kinds, or
	// returns (nil, nil) when none is available.
	Claim(ctx context.Context, kinds []string, lease time.Duration) (*Job, error)
	Complete(ctx context.Context, id string, result json.RawMessage) error
	// Fail records a failure. When retryAt is nil the job fails permanently.
	Fail(ctx context.Context, id string, msg string, retryAt *time.Time) error
}

// Handler executes a job and returns its JSON result.
type Handler func(ctx context.Context, j Job) (any, error)

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks an error as non-retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// IsPermanent reports whether err was marked non-retryable.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

// Enqueuer creates jobs.
type Enqueuer struct{ store Store }

// NewEnqueuer builds an Enqueuer.
func NewEnqueuer(store Store) *Enqueuer { return &Enqueuer{store: store} }

// Enqueue validates and stores a new job.
func (e *Enqueuer) Enqueue(ctx context.Context, j NewJob) (Job, error) {
	if j.OwnerID == "" || j.Kind == "" {
		return Job{}, errors.New("job owner and kind are required")
	}
	if j.MaxAttempts <= 0 {
		j.MaxAttempts = 3
	}
	input, err := json.Marshal(j.Input)
	if err != nil {
		return Job{}, fmt.Errorf("encode job input: %w", err)
	}
	return e.store.Enqueue(ctx, j, input)
}

// Get returns one job owned by ownerID.
func (e *Enqueuer) Get(ctx context.Context, ownerID, id string) (Job, error) {
	return e.store.Get(ctx, ownerID, id)
}

// RunnerConfig configures a Runner.
type RunnerConfig struct {
	Concurrency  int
	PollInterval time.Duration
	Lease        time.Duration // Must exceed the longest expected job.
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
}

// Runner polls the store and executes jobs with bounded concurrency.
type Runner struct {
	store    Store
	handlers map[string]Handler
	cfg      RunnerConfig
	log      *slog.Logger
}

// NewRunner builds a Runner. Handlers are keyed by job kind.
func NewRunner(store Store, handlers map[string]Handler, cfg RunnerConfig, log *slog.Logger) *Runner {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 15 * time.Minute
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 5 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 5 * time.Minute
	}
	return &Runner{store: store, handlers: handlers, cfg: cfg, log: log}
}

// Run processes jobs until ctx is canceled, then waits for in-flight jobs.
func (r *Runner) Run(ctx context.Context) {
	kinds := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		kinds = append(kinds, k)
	}
	var wg sync.WaitGroup
	for i := range r.cfg.Concurrency {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			r.loop(ctx, worker, kinds)
		}(i)
	}
	wg.Wait()
}

func (r *Runner) loop(ctx context.Context, worker int, kinds []string) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		// Drain available jobs before waiting.
		for ctx.Err() == nil {
			ran, err := r.RunOnce(ctx, kinds)
			if err != nil {
				r.log.Error("job claim failed", "worker", worker, "error", err)
				break
			}
			if !ran {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce claims and executes at most one job. It reports whether a job ran.
func (r *Runner) RunOnce(ctx context.Context, kinds []string) (bool, error) {
	j, err := r.store.Claim(ctx, kinds, r.cfg.Lease)
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}
	if j == nil {
		return false, nil
	}
	r.execute(ctx, *j)
	return true, nil
}

func (r *Runner) execute(ctx context.Context, j Job) {
	log := r.log.With("job_id", j.ID, "kind", j.Kind, "attempt", j.Attempts)
	h, ok := r.handlers[j.Kind]
	if !ok {
		r.finishFailure(ctx, log, j, Permanent(fmt.Errorf("no handler for job kind %q", j.Kind)))
		return
	}
	jobCtx, cancel := context.WithTimeout(ctx, r.cfg.Lease)
	defer cancel()

	start := time.Now()
	result, err := safeCall(jobCtx, h, j)
	if err != nil {
		r.finishFailure(ctx, log, j, err)
		return
	}
	raw, err := json.Marshal(result)
	if err != nil {
		r.finishFailure(ctx, log, j, Permanent(fmt.Errorf("encode job result: %w", err)))
		return
	}
	// Use a fresh context so a shutdown does not lose a finished result.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer saveCancel()
	if err := r.store.Complete(saveCtx, j.ID, raw); err != nil {
		log.Error("failed to record job completion", "error", err)
		return
	}
	log.Info("job succeeded", "duration_ms", time.Since(start).Milliseconds())
}

func safeCall(ctx context.Context, h Handler, j Job) (result any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = Permanent(fmt.Errorf("job handler panicked: %v", p))
		}
	}()
	return h(ctx, j)
}

func (r *Runner) finishFailure(ctx context.Context, log *slog.Logger, j Job, jobErr error) {
	var retryAt *time.Time
	if !IsPermanent(jobErr) && j.Attempts < j.MaxAttempts {
		t := time.Now().Add(Backoff(j.Attempts, r.cfg.BaseBackoff, r.cfg.MaxBackoff))
		retryAt = &t
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := r.store.Fail(saveCtx, j.ID, truncate(jobErr.Error(), 2000), retryAt); err != nil {
		log.Error("failed to record job failure", "error", err)
		return
	}
	if retryAt != nil {
		log.Warn("job failed; will retry", "error", jobErr, "retry_at", retryAt.Format(time.RFC3339))
	} else {
		log.Error("job failed permanently", "error", jobErr)
	}
}

// Backoff returns exponential backoff with full jitter for the given
// attempt number (1-based), capped at max.
func Backoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	// Full jitter in [d/2, d].
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
