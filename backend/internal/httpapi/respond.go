package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"contentfactory/internal/channel"
	"contentfactory/internal/connections"
	"contentfactory/internal/jobs"
	"contentfactory/internal/launchpad"
	"contentfactory/internal/production"
	"contentfactory/internal/rss"
	"contentfactory/internal/trends"
	"contentfactory/internal/youtube"
)

// maxBodyBytes bounds request bodies. Blueprints are the largest payloads.
const maxBodyBytes = 1 << 20

// ErrorBody is the stable error response shape.
type ErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeFieldError(w, status, code, message, "")
}

func writeFieldError(w http.ResponseWriter, status int, code, message, field string) {
	var b ErrorBody
	b.Error.Code, b.Error.Message, b.Error.Field = code, message, field
	writeJSON(w, status, b)
}

// decode reads a JSON body strictly into dst.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		switch {
		case errors.As(err, &mbe):
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Request body is too large.")
		case errors.Is(err, io.EOF):
			writeError(w, http.StatusBadRequest, "invalid_body", "Request body is required.")
		default:
			writeError(w, http.StatusBadRequest, "invalid_body", fmt.Sprintf("Request body is invalid: %v", err))
		}
		return false
	}
	if dec.More() {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must contain a single JSON object.")
		return false
	}
	return true
}

// writeDomainError maps domain errors to HTTP responses. Unknown errors are
// logged and reported generically so internals do not leak.
func (s *server) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *channel.ValidationError
	var te *channel.TransitionError
	var stateErr *production.StateError
	switch {
	case errors.As(err, &ve):
		writeFieldError(w, http.StatusUnprocessableEntity, "validation_failed", ve.Error(), ve.Field)
	case errors.As(err, &te):
		writeError(w, http.StatusConflict, "invalid_transition", te.Error())
	case errors.Is(err, channel.ErrNotFound):
		writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found.")
	case errors.Is(err, channel.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "The channel was modified by another operation. Reload and try again.")
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, "job_not_found", "Job not found.")
	case errors.Is(err, launchpad.ErrSavedNicheNotFound):
		writeError(w, http.StatusNotFound, "saved_niche_not_found", "Saved niche not found.")
	case errors.Is(err, launchpad.ErrSavedNicheExists):
		writeError(w, http.StatusConflict, "saved_niche_exists", err.Error())
	case errors.Is(err, connections.ErrNotFound):
		writeError(w, http.StatusNotFound, "connection_not_found", "This channel has no YouTube connection.")
	case errors.Is(err, connections.ErrStateInvalid):
		writeError(w, http.StatusBadRequest, "invalid_state", err.Error())
	case errors.Is(err, connections.ErrExternalInUse):
		writeError(w, http.StatusConflict, "account_in_use", err.Error())
	case errors.Is(err, youtube.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "integration_not_configured", "The YouTube integration is not configured.")
	case errors.Is(err, youtube.ErrQuotaBudget), errors.Is(err, youtube.ErrQuotaExceeded):
		writeError(w, http.StatusTooManyRequests, "quota_exhausted", "The daily YouTube API quota is used up. It resets at midnight Pacific time.")
	case errors.Is(err, youtube.ErrUnauthorized):
		writeError(w, http.StatusBadGateway, "upstream_unauthorized", "YouTube rejected the credentials. Check the API key or reconnect the account.")
	case errors.Is(err, youtube.ErrUnavailable):
		writeError(w, http.StatusBadGateway, "upstream_unavailable", "YouTube could not be reached. Try again shortly.")
	case errors.Is(err, trends.ErrInvalidQuery):
		writeFieldError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error(), "query")
	case errors.As(err, &stateErr):
		writeError(w, http.StatusConflict, "invalid_state", stateErr.Error())
	case errors.Is(err, production.ErrNotFound):
		writeError(w, http.StatusNotFound, "video_not_found", "Video or file not found.")
	case errors.Is(err, production.ErrExists):
		writeError(w, http.StatusConflict, "video_exists", err.Error())
	case errors.Is(err, production.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "The video was modified by another operation. Reload and try again.")
	case errors.Is(err, production.ErrToolsMissing):
		writeError(w, http.StatusServiceUnavailable, "tools_missing", err.Error())
	case errors.Is(err, trends.ErrWatchNotFound):
		writeError(w, http.StatusNotFound, "watch_not_found", "Watched query not found. Channel niches are managed by their channels.")
	case errors.Is(err, rss.ErrFeedNotFound):
		writeError(w, http.StatusNotFound, "feed_not_found", "Feed not found.")
	case errors.Is(err, rss.ErrFeedExists), errors.Is(err, rss.ErrTooManyFeeds):
		writeError(w, http.StatusConflict, "feed_conflict", err.Error())
	case errors.Is(err, rss.ErrInvalidURL):
		writeFieldError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error(), "url")
	case errors.Is(err, launchpad.ErrPrecondition):
		writeError(w, http.StatusConflict, "precondition_failed", err.Error())
	default:
		s.Log.Error("request failed", "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "An unexpected error occurred.")
	}
}
