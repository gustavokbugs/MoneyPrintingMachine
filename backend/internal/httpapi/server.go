// Package httpapi exposes the application over a JSON HTTP API. Handlers
// decode and validate requests, call use cases, and map results and errors
// to stable response shapes. Business rules live in the domain packages.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/launchpad"
	"contentfactory/internal/platforms"
)

// JobQueue enqueues and reads background jobs.
type JobQueue interface {
	Enqueue(ctx context.Context, j jobs.NewJob) (jobs.Job, error)
	Get(ctx context.Context, ownerID, id string) (jobs.Job, error)
}

// SavedNiches stores bookmarked niche ideas.
type SavedNiches interface {
	Create(ctx context.Context, ownerID, name, notes string, data json.RawMessage) (launchpad.SavedNiche, error)
	List(ctx context.Context, ownerID string) ([]launchpad.SavedNiche, error)
	Delete(ctx context.Context, ownerID, id string) error
}

// Connections reads platform connections.
type Connections interface {
	ListForChannel(ctx context.Context, ownerID, channelID string) ([]platforms.Connection, error)
	VerifiedPlatforms(ctx context.Context, ownerID string, channelIDs []string) (map[string][]string, error)
}

// HealthCheck reports the status of one dependency.
type HealthCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Deps holds handler dependencies.
type Deps struct {
	Channels    *channel.Service
	Jobs        JobQueue
	SavedNiches SavedNiches
	Connections Connections
	Health      []HealthCheck
	// Optional integrations; nil when not configured.
	YouTube YouTubeConnections
	Trends  TrendService
	Names   NameChecker
	Quota   QuotaUsage
	// Trend refresh and news feeds; nil disables their endpoints.
	Watchlist  WatchManager
	Feeds      FeedManager
	Trajectory TrajectorySource
	// WebBaseURL is where the browser returns after OAuth.
	WebBaseURL string
	Model      string
	Log        *slog.Logger
	// OwnerID identifies the single local operator. Multi-user
	// authentication is not implemented yet; every request acts as this
	// owner, and all data access is still owner-scoped.
	OwnerID string
}

type server struct{ Deps }

// NewHandler builds the API handler with middleware applied.
func NewHandler(d Deps) http.Handler {
	s := &server{Deps: d}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.health)

	mux.HandleFunc("GET /api/channels", s.listChannels)
	mux.HandleFunc("POST /api/channels", s.createChannel)
	mux.HandleFunc("POST /api/channels/bulk/status", s.bulkStatus)
	mux.HandleFunc("GET /api/channels/{id}", s.getChannel)
	mux.HandleFunc("PATCH /api/channels/{id}", s.updateChannel)
	mux.HandleFunc("POST /api/channels/{id}/status", s.changeStatus)
	mux.HandleFunc("PUT /api/channels/{id}/strategy", s.putStrategy)
	mux.HandleFunc("PUT /api/channels/{id}/identity", s.putIdentity)
	mux.HandleFunc("PUT /api/channels/{id}/blueprint", s.putBlueprint)
	mux.HandleFunc("POST /api/channels/{id}/strategy/generate", s.generate(launchpad.KindGenerateStrategy))
	mux.HandleFunc("POST /api/channels/{id}/identity/generate", s.generate(launchpad.KindGenerateIdentity))
	mux.HandleFunc("POST /api/channels/{id}/blueprint/generate", s.generate(launchpad.KindGenerateBlueprint))
	mux.HandleFunc("GET /api/channels/{id}/assets/{asset}", s.asset)
	mux.HandleFunc("GET /api/channels/{id}/connections", s.listConnections)
	mux.HandleFunc("GET /api/channels/{id}/launch-review", s.launchReview)

	mux.HandleFunc("POST /api/launchpad/niches/discover", s.discoverNiches)
	mux.HandleFunc("GET /api/launchpad/saved-niches", s.listSavedNiches)
	mux.HandleFunc("POST /api/launchpad/saved-niches", s.createSavedNiche)
	mux.HandleFunc("DELETE /api/launchpad/saved-niches/{id}", s.deleteSavedNiche)

	mux.HandleFunc("POST /api/channels/{id}/identity/check-names", s.checkNames)
	mux.HandleFunc("POST /api/channels/{id}/connections/youtube/start", s.startYouTube)
	mux.HandleFunc("POST /api/channels/{id}/connections/youtube/verify", s.verifyYouTube)
	mux.HandleFunc("DELETE /api/channels/{id}/connections/youtube", s.disconnectYouTube)
	mux.HandleFunc("GET /api/oauth/youtube/callback", s.youtubeCallback)

	mux.HandleFunc("POST /api/trends/measure", s.measureTrend)
	mux.HandleFunc("GET /api/trends/history", s.trendHistory)
	mux.HandleFunc("GET /api/integrations", s.integrations)
	if d.Watchlist != nil && d.Feeds != nil && d.Trajectory != nil {
		mux.HandleFunc("GET /api/trends/watchlist", s.listWatches)
		mux.HandleFunc("POST /api/trends/watchlist", s.addWatch)
		mux.HandleFunc("DELETE /api/trends/watchlist/{id}", s.deleteWatch)
		mux.HandleFunc("GET /api/trends/trajectory", s.trajectory)
		mux.HandleFunc("GET /api/trends/feeds", s.listFeeds)
		mux.HandleFunc("POST /api/trends/feeds", s.addFeed)
		mux.HandleFunc("DELETE /api/trends/feeds/{id}", s.deleteFeed)
		mux.HandleFunc("GET /api/trends/mentions", s.mentions)
		mux.HandleFunc("POST /api/trends/refresh", s.refreshNow)
	}

	mux.HandleFunc("GET /api/platforms", s.listPlatforms)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint not found.")
	})

	return s.recoverer(s.logRequests(securityHeaders(mux)))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		// Query strings are not logged; they may contain user input.
		s.Log.Info("http request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				s.Log.Error("panic in handler", "path", r.URL.Path, "panic", p)
				writeError(w, http.StatusInternalServerError, "internal", "An unexpected error occurred.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	type dep struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	resp := struct {
		Status       string `json:"status"`
		Model        string `json:"model"`
		Dependencies []dep  `json:"dependencies"`
	}{Status: "ok", Model: s.Model}
	for _, hc := range s.Health {
		d := dep{Name: hc.Name, Status: "ok"}
		if err := hc.Check(ctx); err != nil {
			d.Status, d.Error = "unavailable", err.Error()
			resp.Status = "degraded"
		}
		resp.Dependencies = append(resp.Dependencies, d)
	}
	writeJSON(w, http.StatusOK, resp)
}
