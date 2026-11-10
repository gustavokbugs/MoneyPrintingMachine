package httpapi

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"contentfactory/internal/branding"
	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/launchpad"
	"contentfactory/internal/platforms"
)

// channelResponse is the API representation of a channel.
type channelResponse struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Status             channel.Status     `json:"status"`
	StatusReason       string             `json:"statusReason"`
	AllowedTransitions []channel.Status   `json:"allowedTransitions"`
	Niche              string             `json:"niche"`
	Language           string             `json:"language"`
	GeoTarget          string             `json:"geoTarget"`
	Strategy           *channel.Strategy  `json:"strategy"`
	Identity           *channel.Identity  `json:"identity"`
	Blueprint          *channel.Blueprint `json:"blueprint"`
	// ConnectedPlatforms lists platforms with a verified connection. It is
	// included only in list responses; omitted means none.
	ConnectedPlatforms []string  `json:"connectedPlatforms,omitempty"`
	Version            int       `json:"version"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func toResponse(c *channel.Channel) channelResponse {
	return channelResponse{
		ID: c.ID, Name: c.Name, Status: c.Status, StatusReason: c.StatusReason,
		AllowedTransitions: channel.AllowedTransitions(c.Status),
		Niche:              c.Niche, Language: c.Language, GeoTarget: c.GeoTarget,
		Strategy: c.Strategy, Identity: c.Identity, Blueprint: c.Blueprint,
		Version: c.Version, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func (s *server) listChannels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 25
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeFieldError(w, http.StatusBadRequest, "invalid_query", "limit must be between 1 and 100.", "limit")
			return
		}
		limit = n
	}
	items, next, err := s.Channels.List(r.Context(), s.OwnerID, channel.Page{Limit: limit, Cursor: q.Get("cursor")})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	verified, err := s.Connections.VerifiedPlatforms(r.Context(), s.OwnerID, ids)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	out := make([]channelResponse, 0, len(items))
	for i := range items {
		resp := toResponse(&items[i])
		resp.ConnectedPlatforms = verified[items[i].ID]
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "nextCursor": next})
}

type createChannelRequest struct {
	Niche     string `json:"niche"`
	Language  string `json:"language"`
	GeoTarget string `json:"geoTarget"`
}

func (s *server) createChannel(w http.ResponseWriter, r *http.Request) {
	var req createChannelRequest
	if !decode(w, r, &req) {
		return
	}
	c, err := s.Channels.CreateDraft(r.Context(), s.OwnerID, req.Niche, req.Language, req.GeoTarget)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(c))
}

func (s *server) getChannel(w http.ResponseWriter, r *http.Request) {
	c, err := s.Channels.Get(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(c))
}

type updateChannelRequest struct {
	Name      *string `json:"name"`
	Niche     *string `json:"niche"`
	Language  *string `json:"language"`
	GeoTarget *string `json:"geoTarget"`
}

func (s *server) updateChannel(w http.ResponseWriter, r *http.Request) {
	var req updateChannelRequest
	if !decode(w, r, &req) {
		return
	}
	c, err := s.Channels.UpdateSettings(r.Context(), s.OwnerID, r.PathValue("id"), channel.SettingsUpdate{
		Name: req.Name, Niche: req.Niche, Language: req.Language, GeoTarget: req.GeoTarget,
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(c))
}

type statusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func (s *server) changeStatus(w http.ResponseWriter, r *http.Request) {
	var req statusRequest
	if !decode(w, r, &req) {
		return
	}
	to, err := channel.ParseStatus(req.Status)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	c, err := s.Channels.ChangeStatus(r.Context(), s.OwnerID, r.PathValue("id"), to, req.Reason)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(c))
}

type bulkStatusRequest struct {
	IDs    []string `json:"ids"`
	Status string   `json:"status"`
	Reason string   `json:"reason"`
}

func (s *server) bulkStatus(w http.ResponseWriter, r *http.Request) {
	var req bulkStatusRequest
	if !decode(w, r, &req) {
		return
	}
	to, err := channel.ParseStatus(req.Status)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	results, err := s.Channels.BulkChangeStatus(r.Context(), s.OwnerID, req.IDs, to, req.Reason)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	type item struct {
		ID     string         `json:"id"`
		OK     bool           `json:"ok"`
		Status channel.Status `json:"status,omitempty"`
		Error  string         `json:"error,omitempty"`
	}
	out := make([]item, 0, len(results))
	for _, res := range results {
		it := item{ID: res.ID, OK: res.Err == nil, Status: res.Status}
		if res.Err != nil {
			it.Error = res.Err.Error()
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

func (s *server) putStrategy(w http.ResponseWriter, r *http.Request) {
	var st channel.Strategy
	if !decode(w, r, &st) {
		return
	}
	st.Provenance.EditedByUser = true
	c, err := s.Channels.SetStrategy(r.Context(), s.OwnerID, r.PathValue("id"), st)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(c))
}

func (s *server) putIdentity(w http.ResponseWriter, r *http.Request) {
	var idn channel.Identity
	if !decode(w, r, &idn) {
		return
	}
	c, err := s.Channels.EditIdentity(r.Context(), s.OwnerID, r.PathValue("id"), idn)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(c))
}

func (s *server) putBlueprint(w http.ResponseWriter, r *http.Request) {
	var bp channel.Blueprint
	if !decode(w, r, &bp) {
		return
	}
	bp.Provenance.EditedByUser = true
	c, err := s.Channels.SetBlueprint(r.Context(), s.OwnerID, r.PathValue("id"), bp)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(c))
}

var idempotencyKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`)

func idempotencyKey(w http.ResponseWriter, r *http.Request, scope string) (string, bool) {
	k := r.Header.Get("Idempotency-Key")
	if k == "" {
		return "", true
	}
	if !idempotencyKeyRe.MatchString(k) {
		writeFieldError(w, http.StatusBadRequest, "invalid_header", "Idempotency-Key must be 8-100 letters, digits, '-' or '_'.", "Idempotency-Key")
		return "", false
	}
	return scope + ":" + k, true
}

// generate enqueues a channel generation job after checking ownership and
// preconditions, so obviously invalid requests fail fast.
func (s *server) generate(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := s.Channels.Get(r.Context(), s.OwnerID, r.PathValue("id"))
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		if c.Status == channel.StatusArchived {
			writeError(w, http.StatusConflict, "channel_archived", "Archived channels cannot be regenerated.")
			return
		}
		switch {
		case kind == launchpad.KindGenerateStrategy && c.Niche == "":
			writeError(w, http.StatusConflict, "precondition_failed", "Select a niche before generating a strategy.")
			return
		case kind != launchpad.KindGenerateStrategy && c.Strategy == nil:
			writeError(w, http.StatusConflict, "precondition_failed", "Generate or write a strategy first.")
			return
		}
		key, ok := idempotencyKey(w, r, kind+":"+c.ID)
		if !ok {
			return
		}
		j, err := s.Jobs.Enqueue(r.Context(), jobs.NewJob{
			OwnerID: s.OwnerID, Kind: kind, ChannelID: c.ID, Input: map[string]string{}, IdempotencyKey: key,
		})
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, j)
	}
}

func (s *server) asset(w http.ResponseWriter, r *http.Request) {
	asset := r.PathValue("asset")
	if asset != "avatar.svg" && asset != "banner.svg" {
		writeError(w, http.StatusNotFound, "asset_not_found", "Asset not found.")
		return
	}
	variant := 0
	if v := r.URL.Query().Get("variant"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n >= branding.MaxVariant {
			writeFieldError(w, http.StatusBadRequest, "invalid_query", "variant is out of range.", "variant")
			return
		}
		variant = n
	}
	c, err := s.Channels.Get(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	if c.Identity == nil {
		writeError(w, http.StatusConflict, "precondition_failed", "Generate a channel identity first.")
		return
	}
	name := c.Name
	if name == "" && len(c.Identity.NameSuggestions) > 0 {
		name = c.Identity.NameSuggestions[0].Name
	}
	kind := strings.TrimSuffix(asset, ".svg")
	etag := `"` + branding.ContentKey(kind, *c.Identity, name, branding.Variant(variant)) + `"`
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "private, max-age=0, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	var svg string
	if kind == "avatar" {
		svg = branding.Avatar(*c.Identity, name, branding.Variant(variant))
	} else {
		svg = branding.Banner(*c.Identity, name, branding.Variant(variant))
	}
	h.Set("Content-Type", "image/svg+xml")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(svg))
}

func (s *server) listConnections(w http.ResponseWriter, r *http.Request) {
	c, err := s.Channels.Get(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	conns, err := s.Connections.ListForChannel(r.Context(), s.OwnerID, c.ID)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": conns})
}

type checkItem struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func (s *server) launchReview(w http.ResponseWriter, r *http.Request) {
	c, err := s.Channels.Get(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	conns, err := s.Connections.ListForChannel(r.Context(), s.OwnerID, c.ID)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	verified, canPublish := 0, 0
	for _, cn := range conns {
		if cn.Status == "verified" {
			verified++
			if cn.CanPublish {
				canPublish++
			}
		}
	}
	checks := []checkItem{
		{ID: "niche", Label: "Niche selected", OK: c.Niche != "", Detail: c.Niche},
		{ID: "strategy", Label: "Channel strategy", OK: c.Strategy != nil},
		{ID: "name", Label: "Channel name selected", OK: c.Name != "", Detail: c.Name},
		{ID: "identity", Label: "Brand identity", OK: c.Identity != nil},
		{ID: "blueprint", Label: "Content blueprint", OK: c.Blueprint != nil},
		{ID: "platforms", Label: "Verified platform connection", OK: verified > 0,
			Detail: "Platform connections are verified through official OAuth integrations. None are verified yet."},
		{ID: "publishing", Label: "Publishing permission", OK: canPublish > 0},
	}
	if verified > 0 {
		checks[5].Detail = strconv.Itoa(verified) + " verified connection(s)."
	}
	ready := true
	for _, ch := range checks {
		ready = ready && ch.OK
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"channel":       toResponse(c),
		"connections":   conns,
		"checks":        checks,
		"readyToLaunch": ready,
	})
}

func (s *server) listPlatforms(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": platforms.Catalog(s.YouTube != nil && s.YouTube.Configured())})
}

func (s *server) getJob(w http.ResponseWriter, r *http.Request) {
	j, err := s.Jobs.Get(r.Context(), s.OwnerID, r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}
