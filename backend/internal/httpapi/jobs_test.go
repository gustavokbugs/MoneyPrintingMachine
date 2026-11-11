package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
)

type memHistory struct {
	owner   string
	since   time.Time
	limit   int
	exclude []string
}

func (m *memHistory) ListRecent(_ context.Context, owner string, since time.Time, limit int, exclude []string) ([]jobs.Job, error) {
	m.owner, m.since, m.limit, m.exclude = owner, since, limit, exclude
	return []jobs.Job{{ID: "j1", Kind: "launchpad.autopilot", Status: jobs.StatusRunning}}, nil
}

func TestListJobs(t *testing.T) {
	f := newFixture()
	h := &memHistory{}
	srv := NewHandler(Deps{
		Channels: channel.NewService(f.repo, noConnections{}), Jobs: f.jobs, JobHistory: h, HiddenJobKinds: []string{"trends.refresh"},
		SavedNiches: memSaved{}, Connections: noConnections{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), OwnerID: "alice",
	})
	rec := do(t, srv, http.MethodGet, "/api/jobs", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"j1"`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if h.owner != "alice" || h.limit != 50 || len(h.exclude) != 1 || time.Since(h.since) < 23*time.Hour {
		t.Errorf("query = %+v", h)
	}
	// A recent since narrows the window; an old one is capped at 24 hours.
	recent := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if rec := do(t, srv, http.MethodGet, "/api/jobs?since="+recent+"&limit=5", ""); rec.Code != http.StatusOK || h.limit != 5 || time.Since(h.since) > 2*time.Minute {
		t.Errorf("since/limit not applied: %d %+v", rec.Code, h)
	}
	if do(t, srv, http.MethodGet, "/api/jobs?since=2000-01-01T00:00:00Z", ""); time.Since(h.since) > 25*time.Hour {
		t.Errorf("old since not capped: %v", h.since)
	}
	for _, q := range []string{"?since=yesterday", "?limit=0", "?limit=500"} {
		if rec := do(t, srv, http.MethodGet, "/api/jobs"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", q, rec.Code)
		}
	}
	// Without a history the route does not exist.
	if rec := do(t, f.server("alice"), http.MethodGet, "/api/jobs", ""); rec.Code != http.StatusNotFound {
		t.Errorf("no history: status %d", rec.Code)
	}
}
