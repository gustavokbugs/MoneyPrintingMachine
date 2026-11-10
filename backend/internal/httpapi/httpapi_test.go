package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/launchpad"
	"contentfactory/internal/platforms"
)

// memChannels is an in-memory channel.Repository shared by test servers.
type memChannels struct {
	mu    sync.Mutex
	items map[string]channel.Channel
	n     int
}

func (m *memChannels) Create(_ context.Context, c *channel.Channel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.n++
	c.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", m.n)
	c.Version = 1
	m.items[c.ID] = *c
	return nil
}

func (m *memChannels) Get(_ context.Context, owner, id string) (*channel.Channel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.items[id]
	if !ok || c.OwnerID != owner {
		return nil, channel.ErrNotFound
	}
	return &c, nil
}

func (m *memChannels) List(_ context.Context, owner string, _ channel.Page) ([]channel.Channel, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []channel.Channel
	for _, c := range m.items {
		if c.OwnerID == owner {
			out = append(out, c)
		}
	}
	return out, "", nil
}

func (m *memChannels) Save(_ context.Context, c *channel.Channel, _ *channel.StatusEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.items[c.ID]
	if !ok || stored.OwnerID != c.OwnerID {
		return channel.ErrNotFound
	}
	if stored.Version != c.Version {
		return channel.ErrConflict
	}
	c.Version++
	m.items[c.ID] = *c
	return nil
}

type noConnections struct{}

func (noConnections) CountVerified(context.Context, string, string) (int, error) { return 0, nil }
func (noConnections) ListForChannel(context.Context, string, string) ([]platforms.Connection, error) {
	return []platforms.Connection{}, nil
}

type memJobs struct {
	mu      sync.Mutex
	created []jobs.NewJob
}

func (m *memJobs) Enqueue(_ context.Context, j jobs.NewJob) (jobs.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, j)
	return jobs.Job{ID: fmt.Sprintf("job-%d", len(m.created)), Kind: j.Kind, ChannelID: j.ChannelID, Status: jobs.StatusQueued}, nil
}

func (m *memJobs) Get(_ context.Context, owner, id string) (jobs.Job, error) {
	return jobs.Job{}, jobs.ErrNotFound
}

type memSaved struct{}

func (memSaved) Create(_ context.Context, _, name, notes string, _ json.RawMessage) (launchpad.SavedNiche, error) {
	if name == "Taken" {
		return launchpad.SavedNiche{}, launchpad.ErrSavedNicheExists
	}
	return launchpad.SavedNiche{ID: "n1", Name: name, Notes: notes}, nil
}
func (memSaved) List(context.Context, string) ([]launchpad.SavedNiche, error) {
	return []launchpad.SavedNiche{}, nil
}
func (memSaved) Delete(context.Context, string, string) error { return launchpad.ErrSavedNicheNotFound }

type fixture struct {
	repo *memChannels
	jobs *memJobs
}

func newFixture() *fixture {
	return &fixture{repo: &memChannels{items: map[string]channel.Channel{}}, jobs: &memJobs{}}
}

func (f *fixture) server(owner string) http.Handler {
	return NewHandler(Deps{
		Channels:    channel.NewService(f.repo, noConnections{}),
		Jobs:        f.jobs,
		SavedNiches: memSaved{},
		Connections: noConnections{},
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OwnerID:     owner,
	})
}

func do(t *testing.T, h http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("response is not an error body: %s", rec.Body.String())
	}
	return b.Error.Code
}

func createChannel(t *testing.T, h http.Handler) channelResponse {
	t.Helper()
	rec := do(t, h, "POST", "/api/channels", `{"niche":"Space history"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var c channelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCreateAndGetChannel(t *testing.T) {
	f := newFixture()
	h := f.server("alice")
	c := createChannel(t, h)
	if c.Status != channel.StatusDraft || c.Language != "en-US" || c.GeoTarget != "US" {
		t.Fatalf("unexpected channel: %+v", c)
	}
	if len(c.AllowedTransitions) == 0 {
		t.Fatal("allowed transitions missing")
	}
	rec := do(t, h, "GET", "/api/channels/"+c.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	rec = do(t, h, "GET", "/api/channels", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), c.ID) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
}

func TestChannelOwnershipIsEnforced(t *testing.T) {
	f := newFixture()
	c := createChannel(t, f.server("alice"))
	bob := f.server("bob")
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/channels/" + c.ID, ""},
		{"PATCH", "/api/channels/" + c.ID, `{"name":"Mine"}`},
		{"POST", "/api/channels/" + c.ID + "/status", `{"status":"archived"}`},
		{"POST", "/api/channels/" + c.ID + "/strategy/generate", ""},
		{"GET", "/api/channels/" + c.ID + "/assets/avatar.svg", ""},
		{"GET", "/api/channels/" + c.ID + "/launch-review", ""},
	} {
		rec := do(t, bob, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusNotFound || errorCode(t, rec) != "channel_not_found" {
			t.Errorf("%s %s as other owner: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	if len(f.jobs.created) != 0 {
		t.Fatal("no job may be enqueued for another owner's channel")
	}
}

func TestInvalidRequests(t *testing.T) {
	f := newFixture()
	h := f.server("alice")
	c := createChannel(t, h)
	tests := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"unknown field", "POST", "/api/channels", `{"niche":"x","admin":true}`, 400, "invalid_body"},
		{"empty body", "POST", "/api/channels", ``, 400, "invalid_body"},
		{"trailing data", "POST", "/api/channels", `{"niche":"x"}{"niche":"y"}`, 400, "invalid_body"},
		{"bad language", "POST", "/api/channels", `{"language":"english"}`, 422, "validation_failed"},
		{"unknown status", "POST", "/api/channels/" + c.ID + "/status", `{"status":"deleted"}`, 422, "validation_failed"},
		{"disallowed transition", "POST", "/api/channels/" + c.ID + "/status", `{"status":"active"}`, 409, "invalid_transition"},
		{"branding needs strategy", "POST", "/api/channels/" + c.ID + "/status", `{"status":"branding"}`, 409, "invalid_transition"},
		{"identity needs strategy", "POST", "/api/channels/" + c.ID + "/identity/generate", ``, 409, "precondition_failed"},
		{"bad limit", "GET", "/api/channels?limit=1000", ``, 400, "invalid_query"},
		{"asset needs identity", "GET", "/api/channels/" + c.ID + "/assets/avatar.svg", ``, 409, "precondition_failed"},
		{"unknown asset", "GET", "/api/channels/" + c.ID + "/assets/logo.png", ``, 404, "asset_not_found"},
		{"bulk archive rejected", "POST", "/api/channels/bulk/status", `{"ids":["` + c.ID + `"],"status":"archived"}`, 422, "validation_failed"},
		{"unknown endpoint", "GET", "/api/nope", ``, 404, "not_found"},
		{"long niche query", "POST", "/api/launchpad/niches/discover", `{"query":"` + strings.Repeat("x", 201) + `"}`, 422, "validation_failed"},
		{"duplicate saved niche", "POST", "/api/launchpad/saved-niches", `{"name":"Taken"}`, 409, "saved_niche_exists"},
		{"missing saved niche", "DELETE", "/api/launchpad/saved-niches/abc", ``, 404, "saved_niche_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, tt.body)
			if rec.Code != tt.status || errorCode(t, rec) != tt.code {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body.String(), tt.status, tt.code)
			}
		})
	}
}

func TestGenerateEnqueuesJobWithIdempotencyKey(t *testing.T) {
	f := newFixture()
	h := f.server("alice")
	c := createChannel(t, h)
	rec := do(t, h, "POST", "/api/channels/"+c.ID+"/strategy/generate", "", "Idempotency-Key", "click-12345678")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("generate: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.jobs.created) != 1 {
		t.Fatalf("expected one job, got %d", len(f.jobs.created))
	}
	j := f.jobs.created[0]
	if j.Kind != launchpad.KindGenerateStrategy || j.ChannelID != c.ID || j.OwnerID != "alice" ||
		j.IdempotencyKey != launchpad.KindGenerateStrategy+":"+c.ID+":click-12345678" {
		t.Fatalf("unexpected job: %+v", j)
	}
	rec = do(t, h, "POST", "/api/channels/"+c.ID+"/strategy/generate", "", "Idempotency-Key", "bad key!")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid idempotency key to be rejected, got %d", rec.Code)
	}
}

func identityFixture() channel.Identity {
	return channel.Identity{
		NameSuggestions: []channel.NameSuggestion{{Name: "Quiet Orbit", Availability: "available"}},
		ShortBio:        "Bio", FullDescription: "Description", Tagline: "<b>Tag</b>",
		Palette: channel.Palette{Primary: "#1D3557", Secondary: "#457B9D", Accent: "#E63946", Background: "#F1FAEE", Text: "#111111"},
	}
}

func TestIdentityEditAndAssets(t *testing.T) {
	f := newFixture()
	h := f.server("alice")
	c := createChannel(t, h)
	body, _ := json.Marshal(identityFixture())
	rec := do(t, h, "PUT", "/api/channels/"+c.ID+"/identity", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("put identity: %d %s", rec.Code, rec.Body.String())
	}
	var got channelResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Identity.NameSuggestions[0].Availability != "not_checked" {
		t.Fatal("clients must not be able to claim name availability")
	}
	if !got.Identity.Provenance.EditedByUser {
		t.Fatal("user edits must be recorded in provenance")
	}

	bad := identityFixture()
	bad.Palette.Accent = "javascript:alert(1)"
	body, _ = json.Marshal(bad)
	if rec := do(t, h, "PUT", "/api/channels/"+c.ID+"/identity", string(body)); rec.Code != 422 {
		t.Fatalf("invalid palette accepted: %d", rec.Code)
	}

	rec = do(t, h, "GET", "/api/channels/"+c.ID+"/assets/banner.svg?variant=2", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("banner: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatal("SVG responses need a restrictive CSP")
	}
	if strings.Contains(rec.Body.String(), "<b>Tag</b>") {
		t.Fatal("tagline must be escaped")
	}
	etag := rec.Header().Get("ETag")
	rec = do(t, h, "GET", "/api/channels/"+c.ID+"/assets/banner.svg?variant=2", "", "If-None-Match", etag)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/channels/"+c.ID+"/assets/avatar.svg?variant=9", ""); rec.Code != 400 {
		t.Fatalf("expected out-of-range variant to be rejected, got %d", rec.Code)
	}
}

func TestLaunchReviewNeverReportsReadyWithoutVerifiedConnection(t *testing.T) {
	f := newFixture()
	h := f.server("alice")
	c := createChannel(t, h)
	stored := f.repo.items[c.ID]
	idn := identityFixture()
	stored.Name, stored.Identity = "Quiet Orbit", &idn
	stored.Strategy, stored.Blueprint = &channel.Strategy{}, &channel.Blueprint{}
	f.repo.items[c.ID] = stored

	rec := do(t, h, "GET", "/api/channels/"+c.ID+"/launch-review", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("launch review: %d", rec.Code)
	}
	var resp struct {
		ReadyToLaunch bool        `json:"readyToLaunch"`
		Checks        []checkItem `json:"checks"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ReadyToLaunch {
		t.Fatal("channel must not be ready without a verified platform connection")
	}
	for _, ch := range resp.Checks {
		if ch.ID == "platforms" && ch.OK {
			t.Fatal("platform check must fail without verified connections")
		}
	}
}

func TestPlatformsCatalogNeverClaimsReadyIntegration(t *testing.T) {
	h := newFixture().server("alice")
	rec := do(t, h, "GET", "/api/platforms", "")
	var resp struct {
		Items []platforms.Capability `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Items) != 3 {
		t.Fatalf("catalog: %v %s", err, rec.Body.String())
	}
	for _, p := range resp.Items {
		if p.IntegrationReady {
			t.Errorf("%s integration is not implemented and must not be marked ready", p.Platform)
		}
	}
}

func TestDiscoverNichesEnqueuesJob(t *testing.T) {
	f := newFixture()
	h := f.server("alice")
	rec := do(t, h, "POST", "/api/launchpad/niches/discover", `{"query":"vintage watches","geoTarget":"us"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("discover: %d %s", rec.Code, rec.Body.String())
	}
	in := f.jobs.created[0].Input.(launchpad.DiscoverInput)
	if in.Query != "vintage watches" || in.GeoTarget != "US" {
		t.Fatalf("unexpected input: %+v", in)
	}
}
