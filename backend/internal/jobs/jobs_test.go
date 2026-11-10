package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeStore struct {
	next      *Job
	completed json.RawMessage
	failedMsg string
	retryAt   *time.Time
	failed    bool
}

func (f *fakeStore) Enqueue(context.Context, NewJob, json.RawMessage) (Job, error) {
	return Job{}, nil
}
func (f *fakeStore) Get(context.Context, string, string) (Job, error) { return Job{}, ErrNotFound }
func (f *fakeStore) Claim(context.Context, []string, time.Duration) (*Job, error) {
	j := f.next
	f.next = nil
	return j, nil
}
func (f *fakeStore) Complete(_ context.Context, _ string, r json.RawMessage) error {
	f.completed = r
	return nil
}
func (f *fakeStore) Fail(_ context.Context, _ string, msg string, retryAt *time.Time) error {
	f.failed, f.failedMsg, f.retryAt = true, msg, retryAt
	return nil
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunOnce(t *testing.T) {
	tests := []struct {
		name        string
		attempts    int
		handler     Handler
		wantSuccess bool
		wantRetry   bool
	}{
		{"success", 1, func(context.Context, Job) (any, error) { return map[string]int{"n": 1}, nil }, true, false},
		{"retryable failure", 1, func(context.Context, Job) (any, error) { return nil, errors.New("temporary") }, false, true},
		{"retries exhausted", 3, func(context.Context, Job) (any, error) { return nil, errors.New("temporary") }, false, false},
		{"permanent failure", 1, func(context.Context, Job) (any, error) { return nil, Permanent(errors.New("bad input")) }, false, false},
		{"panic is permanent", 1, func(context.Context, Job) (any, error) { panic("boom") }, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{next: &Job{ID: "j1", Kind: "k", Attempts: tt.attempts, MaxAttempts: 3}}
			r := NewRunner(st, map[string]Handler{"k": tt.handler}, RunnerConfig{}, quietLogger())
			ran, err := r.RunOnce(context.Background(), []string{"k"})
			if err != nil || !ran {
				t.Fatalf("ran=%v err=%v", ran, err)
			}
			if tt.wantSuccess {
				if st.failed || string(st.completed) != `{"n":1}` {
					t.Fatalf("expected success, got failed=%v completed=%s", st.failed, st.completed)
				}
				return
			}
			if !st.failed {
				t.Fatal("expected failure to be recorded")
			}
			if (st.retryAt != nil) != tt.wantRetry {
				t.Fatalf("retry scheduled=%v, want %v", st.retryAt != nil, tt.wantRetry)
			}
		})
	}
}

func TestRunOnceNoJob(t *testing.T) {
	r := NewRunner(&fakeStore{}, map[string]Handler{}, RunnerConfig{}, quietLogger())
	ran, err := r.RunOnce(context.Background(), nil)
	if err != nil || ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
}

func TestUnknownKindFailsPermanently(t *testing.T) {
	st := &fakeStore{next: &Job{ID: "j1", Kind: "unknown", Attempts: 1, MaxAttempts: 3}}
	r := NewRunner(st, map[string]Handler{}, RunnerConfig{}, quietLogger())
	if _, err := r.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !st.failed || st.retryAt != nil {
		t.Fatalf("expected permanent failure, got failed=%v retryAt=%v", st.failed, st.retryAt)
	}
}

func TestBackoffBounds(t *testing.T) {
	base, max := time.Second, 10*time.Second
	for attempt := 1; attempt <= 10; attempt++ {
		for range 50 {
			d := Backoff(attempt, base, max)
			if d < base/2 || d > max {
				t.Fatalf("attempt %d: backoff %v out of bounds", attempt, d)
			}
		}
	}
}
