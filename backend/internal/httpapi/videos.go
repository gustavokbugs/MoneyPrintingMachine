package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"contentfactory/internal/jobs"
	"contentfactory/internal/production"
)

// Videos is the production service used by the API.
type Videos interface {
	CreateFromIdea(ctx context.Context, ownerID, channelID string, ideaIndex int) (*production.Video, error)
	Get(ctx context.Context, ownerID, id string) (*production.Video, error)
	List(ctx context.Context, ownerID, channelID string) ([]production.Video, error)
	EditScript(ctx context.Context, ownerID, id string, s production.Script) (*production.Video, error)
	ReviewClaim(ctx context.Context, ownerID, id string, index int, status, note string) (*production.Video, error)
	Approve(ctx context.Context, ownerID, id string) (*production.Video, error)
	SetVoice(ctx context.Context, ownerID, id string, voice int) (*production.Video, error)
	SetStyle(ctx context.Context, ownerID, id, style string) (*production.Video, error)
	AnimatedScenes() bool
	FootageAvailable() bool
	CheckCanScript(ctx context.Context, ownerID, id string) (*production.Video, error)
	CheckCanProduce(ctx context.Context, ownerID, id string) (*production.Video, error)
	MediaPath(ctx context.Context, ownerID, id, name string) (string, error)
	ToolsMissing() []string
}

func (s *server) productionStatus(w http.ResponseWriter, _ *http.Request) {
	missing := s.Videos.ToolsMissing()
	if missing == nil {
		missing = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": len(missing) == 0, "missing": missing, "voices": production.Voices,
		"styles": production.Styles, "animatedScenes": s.Videos.AnimatedScenes(), "stockFootage": s.Videos.FootageAvailable()})
}

func (s *server) listVideos(w http.ResponseWriter, r *http.Request) {
	items, err := s.Videos.List(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type createVideoRequest struct {
	IdeaIndex int `json:"ideaIndex"`
}

func (s *server) createVideo(w http.ResponseWriter, r *http.Request) {
	var req createVideoRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := s.Videos.CreateFromIdea(r.Context(), s.OwnerID, r.PathValue("id"), req.IdeaIndex)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *server) getVideo(w http.ResponseWriter, r *http.Request) {
	v, err := s.Videos.Get(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) putScript(w http.ResponseWriter, r *http.Request) {
	var sc production.Script
	if !decode(w, r, &sc) {
		return
	}
	v, err := s.Videos.EditScript(r.Context(), s.OwnerID, r.PathValue("id"), sc)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type claimRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (s *server) reviewClaim(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Claim not found.")
		return
	}
	var req claimRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := s.Videos.ReviewClaim(r.Context(), s.OwnerID, r.PathValue("id"), idx, req.Status, req.Note)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) approveVideo(w http.ResponseWriter, r *http.Request) {
	v, err := s.Videos.Approve(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type voiceRequest struct {
	VoiceID int `json:"voiceId"`
}

func (s *server) setVoice(w http.ResponseWriter, r *http.Request) {
	var req voiceRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := s.Videos.SetVoice(r.Context(), s.OwnerID, r.PathValue("id"), req.VoiceID)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) startVideoJob(kind string, check func(context.Context, string, string) (*production.Video, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := check(r.Context(), s.OwnerID, r.PathValue("id"))
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		key, ok := idempotencyKey(w, r, kind+":"+v.ID)
		if !ok {
			return
		}
		j, err := s.Jobs.Enqueue(r.Context(), jobs.NewJob{OwnerID: s.OwnerID, Kind: kind, Input: map[string]string{"videoId": v.ID}, IdempotencyKey: key, MaxAttempts: 2})
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, j)
	}
}

func (s *server) videoMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	p, err := s.Videos.MediaPath(r.Context(), s.OwnerID, r.PathValue("id"), name)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	if name == "video.mp4" {
		w.Header().Set("Content-Type", "video/mp4")
	} else {
		w.Header().Set("Content-Type", "application/x-subrip; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="captions.srt"`)
	}
	w.Header().Set("Cache-Control", "private, no-cache")
	// ServeFile supports range requests, which video players need.
	http.ServeFile(w, r, p)
}

type styleRequest struct {
	VisualStyle string `json:"visualStyle"`
}

func (s *server) setStyle(w http.ResponseWriter, r *http.Request) {
	var req styleRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := s.Videos.SetStyle(r.Context(), s.OwnerID, r.PathValue("id"), req.VisualStyle)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
