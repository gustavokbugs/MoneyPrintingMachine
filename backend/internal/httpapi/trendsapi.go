package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/refresh"
	"contentfactory/internal/rss"
	"contentfactory/internal/trends"
)

// WatchManager manages the trend watchlist.
type WatchManager interface {
	List(ctx context.Context, ownerID string, limit int) ([]trends.Watch, error)
	AddManual(ctx context.Context, w trends.Watch) (trends.Watch, error)
	Delete(ctx context.Context, ownerID string, id int64) error
}

// FeedManager manages RSS feeds and counts mentions.
type FeedManager interface {
	AddFeed(ctx context.Context, ownerID, url string) (rss.FeedRecord, error)
	List(ctx context.Context, ownerID string) ([]rss.FeedRecord, error)
	Delete(ctx context.Context, ownerID string, id int64) error
	Mentions(ctx context.Context, ownerID, query string) (rss.MentionStats, error)
}

// TrajectorySource returns scored measurement history.
type TrajectorySource interface {
	Trajectory(ctx context.Context, ownerID, query, region, language string) ([]trends.Point, error)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not_found", "Resource not found.")
		return 0, false
	}
	return id, true
}

func queryParams(w http.ResponseWriter, r *http.Request) (query, region, language string, ok bool) {
	q := r.URL.Query()
	query = strings.TrimSpace(q.Get("query"))
	if query == "" || len(query) > 200 {
		writeFieldError(w, http.StatusBadRequest, "invalid_query", "query must be 1-200 characters.", "query")
		return "", "", "", false
	}
	region, language = q.Get("region"), q.Get("language")
	if region == "" {
		region = channel.DefaultGeoTarget
	}
	if language == "" {
		language = channel.DefaultLanguage
	}
	return query, region, language, true
}

func (s *server) listWatches(w http.ResponseWriter, r *http.Request) {
	items, err := s.Watchlist.List(r.Context(), s.OwnerID, 200)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type watchRequest struct {
	Query    string `json:"query"`
	Region   string `json:"region"`
	Language string `json:"language"`
}

func (s *server) addWatch(w http.ResponseWriter, r *http.Request) {
	var req watchRequest
	if !decode(w, r, &req) {
		return
	}
	watch, err := trends.ValidateWatch(trends.Watch{OwnerID: s.OwnerID, Query: req.Query, Region: req.Region, Language: req.Language})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	watch.OwnerID = s.OwnerID
	out, err := s.Watchlist.AddManual(r.Context(), watch)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *server) deleteWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Watchlist.Delete(r.Context(), s.OwnerID, id); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) trajectory(w http.ResponseWriter, r *http.Request) {
	query, region, language, ok := queryParams(w, r)
	if !ok {
		return
	}
	points, err := s.Trajectory.Trajectory(r.Context(), s.OwnerID, query, region, language)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": points})
}

func (s *server) listFeeds(w http.ResponseWriter, r *http.Request) {
	items, err := s.Feeds.List(r.Context(), s.OwnerID)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type feedRequest struct {
	URL string `json:"url"`
}

func (s *server) addFeed(w http.ResponseWriter, r *http.Request) {
	var req feedRequest
	if !decode(w, r, &req) {
		return
	}
	f, err := s.Feeds.AddFeed(r.Context(), s.OwnerID, req.URL)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *server) deleteFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Feeds.Delete(r.Context(), s.OwnerID, id); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) mentions(w http.ResponseWriter, r *http.Request) {
	query, _, _, ok := queryParams(w, r)
	if !ok {
		return
	}
	st, err := s.Feeds.Mentions(r.Context(), s.OwnerID, query)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// refreshNow enqueues an immediate refresh run.
func (s *server) refreshNow(w http.ResponseWriter, r *http.Request) {
	key, ok := idempotencyKey(w, r, refresh.KindTrendRefresh)
	if !ok {
		return
	}
	j, err := s.Jobs.Enqueue(r.Context(), jobs.NewJob{OwnerID: s.OwnerID, Kind: refresh.KindTrendRefresh, Input: map[string]string{}, IdempotencyKey: key, MaxAttempts: 2})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, j)
}
