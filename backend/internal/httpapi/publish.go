package httpapi

import (
	"context"
	"net/http"

	"contentfactory/internal/jobs"
	"contentfactory/internal/production"
	"contentfactory/internal/publishing"
)

// Publisher uploads produced videos.
type Publisher interface {
	Check(ctx context.Context, ownerID, videoID, privacy string) (*production.Video, error)
	List(ctx context.Context, ownerID, videoID string) ([]publishing.Publication, error)
}

type publishRequest struct {
	Privacy string `json:"privacy"`
}

func (s *server) publishVideo(w http.ResponseWriter, r *http.Request) {
	var req publishRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := s.Publisher.Check(r.Context(), s.OwnerID, r.PathValue("id"), req.Privacy)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	key, ok := idempotencyKey(w, r, publishing.KindYouTube+":"+v.ID)
	if !ok {
		return
	}
	j, err := s.Jobs.Enqueue(r.Context(), jobs.NewJob{OwnerID: s.OwnerID, Kind: publishing.KindYouTube,
		Input: map[string]string{"videoId": v.ID, "privacy": req.Privacy}, IdempotencyKey: key, MaxAttempts: 1})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, j)
}

func (s *server) listPublications(w http.ResponseWriter, r *http.Request) {
	items, err := s.Publisher.List(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
