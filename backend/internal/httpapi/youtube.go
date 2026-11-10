package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/connections"
	"contentfactory/internal/platforms"
	"contentfactory/internal/trends"
)

// YouTubeConnections manages YouTube OAuth connections.
type YouTubeConnections interface {
	Configured() bool
	Start(ctx context.Context, ownerID, channelID string) (string, error)
	Callback(ctx context.Context, ownerID, state, code, oauthError string) (connections.CallbackResult, error)
	Verify(ctx context.Context, ownerID, channelID string) (*platforms.Connection, error)
	Disconnect(ctx context.Context, ownerID, channelID string) error
}

// TrendService measures queries.
type TrendService interface {
	Available() bool
	Measure(ctx context.Context, r trends.Request) (trends.Result, error)
	History(ctx context.Context, ownerID, query, region, language string, limit int) ([]trends.Snapshot, error)
}

// NameChecker checks name suggestions against YouTube handles.
type NameChecker interface {
	HandleChecksAvailable() bool
	CheckNames(ctx context.Context, ownerID, channelID string) (*channel.Channel, error)
}

// QuotaUsage reports today's YouTube quota usage.
type QuotaUsage interface {
	Usage(ctx context.Context) (used, limit int, err error)
}

func (s *server) integrations(w http.ResponseWriter, r *http.Request) {
	type quota struct {
		UsedToday int `json:"usedToday"`
		Limit     int `json:"limit"`
	}
	resp := struct {
		YouTubeDataAPI struct {
			Configured bool   `json:"configured"`
			Quota      *quota `json:"quota"`
		} `json:"youtubeDataApi"`
		YouTubeOAuth struct {
			Configured bool `json:"configured"`
		} `json:"youtubeOAuth"`
	}{}
	resp.YouTubeDataAPI.Configured = s.Trends != nil && s.Trends.Available()
	resp.YouTubeOAuth.Configured = s.YouTube != nil && s.YouTube.Configured()
	if s.Quota != nil {
		if used, limit, err := s.Quota.Usage(r.Context()); err == nil {
			resp.YouTubeDataAPI.Quota = &quota{UsedToday: used, Limit: limit}
		} else {
			s.Log.Warn("read quota usage", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type measureRequest struct {
	Query    string `json:"query"`
	Region   string `json:"region"`
	Language string `json:"language"`
}

func (s *server) measureTrend(w http.ResponseWriter, r *http.Request) {
	if s.Trends == nil {
		writeError(w, http.StatusServiceUnavailable, "integration_not_configured", "Trend measurement is not configured.")
		return
	}
	var req measureRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Region == "" {
		req.Region = channel.DefaultGeoTarget
	}
	if req.Language == "" {
		req.Language = channel.DefaultLanguage
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.Trends.Measure(ctx, trends.Request{OwnerID: s.OwnerID, Query: req.Query, Region: req.Region, Language: req.Language})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) trendHistory(w http.ResponseWriter, r *http.Request) {
	if s.Trends == nil {
		writeError(w, http.StatusServiceUnavailable, "integration_not_configured", "Trend measurement is not configured.")
		return
	}
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("query"))
	if query == "" || len(query) > 200 {
		writeFieldError(w, http.StatusBadRequest, "invalid_query", "query must be 1-200 characters.", "query")
		return
	}
	region, lang := q.Get("region"), q.Get("language")
	if region == "" {
		region = channel.DefaultGeoTarget
	}
	if lang == "" {
		lang = channel.DefaultLanguage
	}
	items, err := s.Trends.History(r.Context(), s.OwnerID, query, region, lang, 50)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) checkNames(w http.ResponseWriter, r *http.Request) {
	if s.Names == nil || !s.Names.HandleChecksAvailable() {
		writeError(w, http.StatusServiceUnavailable, "integration_not_configured", "Set YOUTUBE_API_KEY to check channel handles.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	c, err := s.Names.CheckNames(ctx, s.OwnerID, r.PathValue("id"))
	if err != nil && c == nil {
		s.writeDomainError(w, r, err)
		return
	}
	resp := map[string]any{"channel": toResponse(c), "complete": err == nil}
	if err != nil {
		resp["warning"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) youtubeConfigured(w http.ResponseWriter) bool {
	if s.YouTube == nil || !s.YouTube.Configured() {
		writeError(w, http.StatusServiceUnavailable, "integration_not_configured",
			"YouTube connections need GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, and TOKEN_ENCRYPTION_KEY.")
		return false
	}
	return true
}

func (s *server) startYouTube(w http.ResponseWriter, r *http.Request) {
	if !s.youtubeConfigured(w) {
		return
	}
	authURL, err := s.YouTube.Start(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorizationUrl": authURL})
}

// youtubeCallback finishes OAuth and redirects the browser back to the web
// app. The redirect target is built from configuration and the stored
// channel ID only, so the callback cannot be used as an open redirect.
func (s *server) youtubeCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if s.YouTube == nil {
		http.Redirect(w, r, strings.TrimRight(s.WebBaseURL, "/")+"/launchpad?youtube=error", http.StatusFound)
		return
	}
	result, err := s.YouTube.Callback(r.Context(), s.OwnerID, q.Get("state"), q.Get("code"), q.Get("error"))
	outcome := "connected"
	switch {
	case err == nil:
		if result.Connection != nil && result.Connection.Status != connections.StatusVerified {
			outcome = "needs_attention"
		}
	case errors.Is(err, connections.ErrDenied):
		outcome = "denied"
	case errors.Is(err, connections.ErrStateInvalid):
		outcome = "invalid_state"
	case errors.Is(err, connections.ErrExternalInUse):
		outcome = "already_connected"
	default:
		s.Log.Error("youtube callback failed", "error", err)
		outcome = "error"
	}
	target := strings.TrimRight(s.WebBaseURL, "/") + "/launchpad"
	if result.ChannelID != "" {
		target = strings.TrimRight(s.WebBaseURL, "/") + "/channels/" + url.PathEscape(result.ChannelID)
	}
	target += "?" + url.Values{"step": {"platforms"}, "youtube": {outcome}}.Encode()
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *server) verifyYouTube(w http.ResponseWriter, r *http.Request) {
	if !s.youtubeConfigured(w) {
		return
	}
	conn, err := s.YouTube.Verify(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, conn)
}

func (s *server) disconnectYouTube(w http.ResponseWriter, r *http.Request) {
	if !s.youtubeConfigured(w) {
		return
	}
	if err := s.YouTube.Disconnect(r.Context(), s.OwnerID, r.PathValue("id")); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
