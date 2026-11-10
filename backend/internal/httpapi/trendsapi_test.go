package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"contentfactory/internal/channel"
	"contentfactory/internal/refresh"
	"contentfactory/internal/rss"
	"contentfactory/internal/trends"
)

type memWatchlist struct{ added []trends.Watch }

func (m *memWatchlist) List(context.Context, string, int) ([]trends.Watch, error) {
	return m.added, nil
}
func (m *memWatchlist) AddManual(_ context.Context, w trends.Watch) (trends.Watch, error) {
	w.ID = int64(len(m.added) + 1)
	m.added = append(m.added, w)
	return w, nil
}
func (m *memWatchlist) Delete(_ context.Context, owner string, id int64) error {
	if owner != "alice" || id != 1 {
		return trends.ErrWatchNotFound
	}
	return nil
}

type memFeeds struct{}

func (memFeeds) AddFeed(_ context.Context, _, url string) (rss.FeedRecord, error) {
	if _, err := rss.ValidateURL(url); err != nil {
		return rss.FeedRecord{}, err
	}
	if url == "https://dup.example/feed" {
		return rss.FeedRecord{}, rss.ErrFeedExists
	}
	return rss.FeedRecord{ID: 9, URL: url, LastStatus: "pending"}, nil
}
func (memFeeds) List(context.Context, string) ([]rss.FeedRecord, error) {
	return []rss.FeedRecord{}, nil
}
func (memFeeds) Delete(context.Context, string, int64) error { return rss.ErrFeedNotFound }
func (memFeeds) Mentions(_ context.Context, _, query string) (rss.MentionStats, error) {
	return rss.MentionStats{Recent7d: len(query), ActiveFeeds: 1}, nil
}

type memTrajectory struct{}

func (memTrajectory) Trajectory(_ context.Context, _, query, region, _ string) ([]trends.Point, error) {
	return []trends.Point{{MedianViewsPerDay: float64(len(query + region))}}, nil
}

func trendServer(f *fixture, wl *memWatchlist) http.Handler {
	return NewHandler(Deps{
		Channels: channel.NewService(f.repo, noConnections{}), Jobs: f.jobs, SavedNiches: memSaved{}, Connections: noConnections{},
		Watchlist: wl, Feeds: memFeeds{}, Trajectory: memTrajectory{},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), OwnerID: "alice",
	})
}

func TestWatchlistEndpoints(t *testing.T) {
	wl := &memWatchlist{}
	h := trendServer(newFixture(), wl)
	rec := do(t, h, "POST", "/api/trends/watchlist", `{"query":"  Deep   Sea ","region":"gb"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	if w := wl.added[0]; w.OwnerID != "alice" || w.Query != "Deep Sea" || w.Region != "GB" || w.Source != trends.WatchSourceManual {
		t.Fatalf("watch not normalized: %+v", w)
	}
	for _, body := range []string{`{"query":""}`, `{"query":"x","region":"USA"}`} {
		if rec := do(t, h, "POST", "/api/trends/watchlist", body); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: expected 422, got %d", body, rec.Code)
		}
	}
	if rec := do(t, h, "DELETE", "/api/trends/watchlist/2", ""); rec.Code != http.StatusNotFound || errorCode(t, rec) != "watch_not_found" {
		t.Fatalf("delete missing: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "DELETE", "/api/trends/watchlist/abc", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("non-numeric id: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/trends/watchlist/1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
}

func TestFeedEndpoints(t *testing.T) {
	h := trendServer(newFixture(), &memWatchlist{})
	if rec := do(t, h, "POST", "/api/trends/feeds", `{"url":"https://news.example/rss"}`); rec.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	tests := []struct {
		body, code string
		status     int
	}{
		{`{"url":"file:///etc/passwd"}`, "validation_failed", 422},
		{`{"url":"https://dup.example/feed"}`, "feed_conflict", 409},
	}
	for _, tt := range tests {
		rec := do(t, h, "POST", "/api/trends/feeds", tt.body)
		if rec.Code != tt.status || errorCode(t, rec) != tt.code {
			t.Errorf("%s: %d %s", tt.body, rec.Code, rec.Body.String())
		}
	}
	if rec := do(t, h, "DELETE", "/api/trends/feeds/5", ""); rec.Code != http.StatusNotFound || errorCode(t, rec) != "feed_not_found" {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	rec := do(t, h, "GET", "/api/trends/mentions?query=deep+sea", "")
	var st rss.MentionStats
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || st.Recent7d != len("deep sea") {
		t.Fatalf("mentions: %s", rec.Body.String())
	}
	if rec := do(t, h, "GET", "/api/trends/mentions", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("mentions without query: %d", rec.Code)
	}
}

func TestTrajectoryAndRefresh(t *testing.T) {
	f := newFixture()
	h := trendServer(f, &memWatchlist{})
	rec := do(t, h, "GET", "/api/trends/trajectory?query=space", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"medianViewsPerDay":7`) {
		t.Fatalf("trajectory defaults region to US: %s", rec.Body.String())
	}
	rec = do(t, h, "POST", "/api/trends/refresh", "", "Idempotency-Key", "refresh-12345678")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}
	j := f.jobs.created[len(f.jobs.created)-1]
	if j.Kind != refresh.KindTrendRefresh || j.OwnerID != "alice" || !strings.HasSuffix(j.IdempotencyKey, "refresh-12345678") {
		t.Fatalf("unexpected job: %+v", j)
	}
}

func TestTrendEndpointsDisabledWithoutDeps(t *testing.T) {
	h := newFixture().server("alice")
	if rec := do(t, h, "GET", "/api/trends/watchlist", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when trend deps are missing, got %d", rec.Code)
	}
}
