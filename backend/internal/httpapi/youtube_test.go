package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"contentfactory/internal/channel"
	"contentfactory/internal/connections"
	"contentfactory/internal/platforms"
	"contentfactory/internal/trends"
	"contentfactory/internal/youtube"
)

type fakeYouTube struct {
	configured  bool
	callbackRes connections.CallbackResult
	callbackErr error
	startOwner  string
}

func (f *fakeYouTube) Configured() bool { return f.configured }
func (f *fakeYouTube) Start(_ context.Context, owner, _ string) (string, error) {
	f.startOwner = owner
	return "https://accounts.google.com/o/oauth2/v2/auth?state=x", nil
}
func (f *fakeYouTube) Callback(context.Context, string, string, string, string) (connections.CallbackResult, error) {
	return f.callbackRes, f.callbackErr
}
func (f *fakeYouTube) Verify(context.Context, string, string) (*platforms.Connection, error) {
	return &platforms.Connection{Platform: "youtube", Status: connections.StatusVerified}, nil
}
func (f *fakeYouTube) Disconnect(context.Context, string, string) error {
	return connections.ErrNotFound
}

type fakeTrends struct{ err error }

func (f fakeTrends) Available() bool { return true }
func (f fakeTrends) Measure(_ context.Context, r trends.Request) (trends.Result, error) {
	if f.err != nil {
		return trends.Result{}, f.err
	}
	return trends.Result{Snapshot: trends.Snapshot{Query: r.Query, Region: r.Region}}, nil
}
func (f fakeTrends) History(context.Context, string, string, string, string, int) ([]trends.Snapshot, error) {
	return []trends.Snapshot{}, nil
}

type fakeQuota struct{}

func (fakeQuota) Usage(context.Context) (int, int, error) { return 204, 10000, nil }

type fakeNames struct{ ch *channel.Channel }

func (f fakeNames) HandleChecksAvailable() bool { return true }
func (f fakeNames) CheckNames(context.Context, string, string) (*channel.Channel, error) {
	return f.ch, fmt.Errorf("some names were not checked: %w", youtube.ErrQuotaBudget)
}

func youtubeServer(f *fixture, yt *fakeYouTube, tr TrendService) http.Handler {
	return NewHandler(Deps{
		Channels:    channel.NewService(f.repo, noConnections{}),
		Jobs:        f.jobs,
		SavedNiches: memSaved{},
		Connections: noConnections{},
		YouTube:     yt,
		Trends:      tr,
		Quota:       fakeQuota{},
		Names:       fakeNames{ch: &channel.Channel{ID: "c", Status: channel.StatusBranding}},
		WebBaseURL:  "http://127.0.0.1:5173",
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OwnerID:     "alice",
	})
}

func TestYouTubeNotConfigured(t *testing.T) {
	f := newFixture()
	h := youtubeServer(f, &fakeYouTube{configured: false}, nil)
	c := createChannel(t, h)
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/channels/" + c.ID + "/connections/youtube/start", ""},
		{"POST", "/api/channels/" + c.ID + "/connections/youtube/verify", ""},
		{"POST", "/api/trends/measure", `{"query":"space"}`},
	} {
		rec := do(t, h, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != "integration_not_configured" {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	rec := do(t, h, "GET", "/api/platforms", "")
	var resp struct {
		Items []platforms.Capability `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Items[0].IntegrationReady {
		t.Fatal("YouTube must not be ready when OAuth is not configured")
	}
}

func TestYouTubeStartAndReadiness(t *testing.T) {
	f := newFixture()
	yt := &fakeYouTube{configured: true}
	h := youtubeServer(f, yt, nil)
	c := createChannel(t, h)
	rec := do(t, h, "POST", "/api/channels/"+c.ID+"/connections/youtube/start", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "authorizationUrl") || yt.startOwner != "alice" {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/api/platforms", "")
	var resp struct {
		Items []platforms.Capability `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Items[0].IntegrationReady || resp.Items[1].IntegrationReady || resp.Items[2].IntegrationReady {
		t.Fatalf("only YouTube should be ready: %+v", resp.Items)
	}
	rec = do(t, h, "DELETE", "/api/channels/"+c.ID+"/connections/youtube", "")
	if rec.Code != http.StatusNotFound || errorCode(t, rec) != "connection_not_found" {
		t.Fatalf("disconnect missing: %d %s", rec.Code, rec.Body.String())
	}
}

func TestYouTubeCallbackRedirects(t *testing.T) {
	tests := []struct {
		name    string
		res     connections.CallbackResult
		err     error
		wantURL string
	}{
		{"verified", connections.CallbackResult{ChannelID: "c1", Connection: &platforms.Connection{Status: connections.StatusVerified}}, nil,
			"http://127.0.0.1:5173/channels/c1?step=platforms&youtube=connected"},
		{"no channel yet", connections.CallbackResult{ChannelID: "c1", Connection: &platforms.Connection{Status: connections.StatusAuthorized}}, nil,
			"http://127.0.0.1:5173/channels/c1?step=platforms&youtube=needs_attention"},
		{"denied", connections.CallbackResult{ChannelID: "c1"}, connections.ErrDenied,
			"http://127.0.0.1:5173/channels/c1?step=platforms&youtube=denied"},
		{"invalid state", connections.CallbackResult{}, connections.ErrStateInvalid,
			"http://127.0.0.1:5173/launchpad?step=platforms&youtube=invalid_state"},
		{"account in use", connections.CallbackResult{ChannelID: "c1"}, connections.ErrExternalInUse,
			"http://127.0.0.1:5173/channels/c1?step=platforms&youtube=already_connected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := youtubeServer(newFixture(), &fakeYouTube{configured: true, callbackRes: tt.res, callbackErr: tt.err}, nil)
			rec := do(t, h, "GET", "/api/oauth/youtube/callback?state=s&code=c&redirect="+url.QueryEscape("https://evil.example"), "")
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != tt.wantURL {
				t.Fatalf("got %d %q, want %q", rec.Code, rec.Header().Get("Location"), tt.wantURL)
			}
		})
	}
}

func TestTrendEndpoints(t *testing.T) {
	f := newFixture()
	h := youtubeServer(f, &fakeYouTube{}, fakeTrends{})
	rec := do(t, h, "POST", "/api/trends/measure", `{"query":"space history"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"region":"US"`) {
		t.Fatalf("measure: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "GET", "/api/trends/history", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("history without query: %d", rec.Code)
	}

	quota := youtubeServer(f, &fakeYouTube{}, fakeTrends{err: fmt.Errorf("search: %w", youtube.ErrQuotaBudget)})
	rec = do(t, quota, "POST", "/api/trends/measure", `{"query":"space"}`)
	if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != "quota_exhausted" {
		t.Fatalf("quota: %d %s", rec.Code, rec.Body.String())
	}
	invalid := youtubeServer(f, &fakeYouTube{}, fakeTrends{err: trends.ErrInvalidQuery})
	if rec := do(t, invalid, "POST", "/api/trends/measure", `{"query":""}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid query: %d", rec.Code)
	}
}

func TestIntegrationsAndCheckNames(t *testing.T) {
	f := newFixture()
	h := youtubeServer(f, &fakeYouTube{configured: true}, fakeTrends{})
	rec := do(t, h, "GET", "/api/integrations", "")
	body := rec.Body.String()
	if !strings.Contains(body, `"usedToday":204`) || !strings.Contains(body, `"youtubeOAuth":{"configured":true}`) {
		t.Fatalf("integrations: %s", body)
	}
	rec = do(t, h, "POST", "/api/channels/c/identity/check-names", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"complete":false`) || !strings.Contains(rec.Body.String(), "warning") {
		t.Fatalf("check names partial: %d %s", rec.Code, rec.Body.String())
	}
}
