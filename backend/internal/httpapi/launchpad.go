package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"contentfactory/internal/jobs"
	"contentfactory/internal/launchpad"
)

type discoverRequest struct {
	Query     string `json:"query"`
	Language  string `json:"language"`
	GeoTarget string `json:"geoTarget"`
}

func (s *server) discoverNiches(w http.ResponseWriter, r *http.Request) {
	var req discoverRequest
	if !decode(w, r, &req) {
		return
	}
	in := launchpad.DiscoverInput{
		Query: strings.TrimSpace(req.Query), Language: req.Language, GeoTarget: strings.ToUpper(req.GeoTarget),
	}
	if err := in.Validate(); err != nil {
		writeFieldError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error(), "query")
		return
	}
	key, ok := idempotencyKey(w, r, launchpad.KindDiscoverNiches)
	if !ok {
		return
	}
	j, err := s.Jobs.Enqueue(r.Context(), jobs.NewJob{
		OwnerID: s.OwnerID, Kind: launchpad.KindDiscoverNiches, Input: in, IdempotencyKey: key,
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, j)
}

func (s *server) listSavedNiches(w http.ResponseWriter, r *http.Request) {
	items, err := s.SavedNiches.List(r.Context(), s.OwnerID)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type savedNicheRequest struct {
	Name  string          `json:"name"`
	Notes string          `json:"notes"`
	Data  json.RawMessage `json:"data"`
}

func (s *server) createSavedNiche(w http.ResponseWriter, r *http.Request) {
	var req savedNicheRequest
	if !decode(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 120 {
		writeFieldError(w, http.StatusUnprocessableEntity, "validation_failed", "name must be 1-120 characters.", "name")
		return
	}
	if len(req.Notes) > 2000 {
		writeFieldError(w, http.StatusUnprocessableEntity, "validation_failed", "notes must be at most 2000 characters.", "notes")
		return
	}
	if len(req.Data) > 0 && !json.Valid(req.Data) {
		writeFieldError(w, http.StatusUnprocessableEntity, "validation_failed", "data must be valid JSON.", "data")
		return
	}
	n, err := s.SavedNiches.Create(r.Context(), s.OwnerID, name, req.Notes, req.Data)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *server) deleteSavedNiche(w http.ResponseWriter, r *http.Request) {
	if err := s.SavedNiches.Delete(r.Context(), s.OwnerID, r.PathValue("id")); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
