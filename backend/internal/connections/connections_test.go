package connections

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/secrets"
	"contentfactory/internal/youtube"
)

type memStore struct {
	states  map[string]State
	records map[string]Record
	// externalOwners maps external IDs to the channel holding them.
	externalOwners map[string]string
}

func newMemStore() *memStore {
	return &memStore{states: map[string]State{}, records: map[string]Record{}, externalOwners: map[string]string{}}
}

func key(owner, ch, p string) string { return owner + "|" + ch + "|" + p }

func (m *memStore) SaveState(_ context.Context, s State) error { m.states[s.StateHash] = s; return nil }
func (m *memStore) ConsumeState(_ context.Context, h string, now time.Time) (State, error) {
	s, ok := m.states[h]
	delete(m.states, h)
	if !ok || !now.Before(s.ExpiresAt) {
		return State{}, ErrStateInvalid
	}
	return s, nil
}
func (m *memStore) Get(_ context.Context, owner, ch, p string) (*Record, error) {
	r, ok := m.records[key(owner, ch, p)]
	if !ok {
		return nil, nil
	}
	return &r, nil
}
func (m *memStore) Upsert(_ context.Context, r *Record) error {
	if r.ExternalID != "" {
		if holder, ok := m.externalOwners[r.ExternalID]; ok && holder != r.ChannelID {
			return ErrExternalInUse
		}
		m.externalOwners[r.ExternalID] = r.ChannelID
	}
	m.records[key(r.OwnerID, r.ChannelID, r.Platform)] = *r
	return nil
}
func (m *memStore) Delete(_ context.Context, owner, ch, p string) error {
	delete(m.records, key(owner, ch, p))
	return nil
}

type fakeOAuth struct {
	tokens     youtube.Token
	refreshed  youtube.Token
	refreshErr error
	revoked    []string
	verifier   string
}

func (f *fakeOAuth) Configured() bool { return true }
func (f *fakeOAuth) AuthCodeURL(state, verifier string, _ []string) string {
	f.verifier = verifier
	return "https://accounts.example/auth?" + url.Values{"state": {state}}.Encode()
}
func (f *fakeOAuth) Exchange(_ context.Context, code, verifier string) (youtube.Token, error) {
	if code != "good" || verifier != f.verifier {
		return youtube.Token{}, errors.New("bad code")
	}
	return f.tokens, nil
}
func (f *fakeOAuth) Refresh(context.Context, string) (youtube.Token, error) {
	return f.refreshed, f.refreshErr
}
func (f *fakeOAuth) Revoke(_ context.Context, tok string) error {
	f.revoked = append(f.revoked, tok)
	return nil
}

type fakeYT struct {
	channels []youtube.ChannelInfo
	err      error
	tokens   []string
}

func (f *fakeYT) MyChannels(_ context.Context, at string) ([]youtube.ChannelInfo, error) {
	f.tokens = append(f.tokens, at)
	return f.channels, f.err
}

type fakeChannels map[string]channel.Channel

func (f fakeChannels) Get(_ context.Context, owner, id string) (*channel.Channel, error) {
	c, ok := f[id]
	if !ok || c.OwnerID != owner {
		return nil, channel.ErrNotFound
	}
	return &c, nil
}

type fixture struct {
	svc   *Service
	store *memStore
	oauth *fakeOAuth
	yt    *fakeYT
	clock time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	k, _ := secrets.GenerateKey()
	box, err := secrets.NewBox(k)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		store: newMemStore(),
		oauth: &fakeOAuth{tokens: youtube.Token{
			AccessToken: "at", RefreshToken: "rt", Expiry: time.Date(2026, 11, 10, 13, 0, 0, 0, time.UTC),
			Scopes: []string{youtube.ScopeReadOnly, youtube.ScopeUpload, youtube.ScopeAnalytics},
		}},
		yt:    &fakeYT{channels: []youtube.ChannelInfo{{ID: "UC123", Title: "Quiet Orbit"}}},
		clock: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
	}
	chans := fakeChannels{
		"ch1": {ID: "ch1", OwnerID: "alice", Status: channel.StatusAwaitingPlatformSetup},
		"ch2": {ID: "ch2", OwnerID: "alice", Status: channel.StatusAwaitingPlatformSetup},
		"old": {ID: "old", OwnerID: "alice", Status: channel.StatusArchived},
	}
	f.svc = NewService(f.store, f.oauth, f.yt, chans, box)
	f.svc.now = func() time.Time { return f.clock }
	return f
}

func stateFrom(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("state")
}

func TestConnectHappyPath(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	authURL, err := f.svc.Start(ctx, "alice", "ch1")
	if err != nil {
		t.Fatal(err)
	}
	state := stateFrom(t, authURL)
	if len(f.store.states) != 1 {
		t.Fatal("state not stored")
	}
	for h := range f.store.states {
		if h == state {
			t.Fatal("raw state must not be stored")
		}
	}
	if rec, _ := f.store.Get(ctx, "alice", "ch1", PlatformYouTube); rec == nil || rec.Status != StatusPending {
		t.Fatalf("expected pending connection, got %+v", rec)
	}

	res, err := f.svc.Callback(ctx, "alice", state, "good", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.ChannelID != "ch1" || res.Connection.Status != StatusVerified || res.Connection.ExternalID != "UC123" ||
		!res.Connection.CanPublish || !res.Connection.CanReadAnalytics {
		t.Fatalf("unexpected result: %+v %+v", res, res.Connection)
	}
	rec, _ := f.store.Get(ctx, "alice", "ch1", PlatformYouTube)
	// AES-GCM output is nonce (12) + plaintext + tag (16); a raw token would be shorter.
	if len(rec.AccessTokenEnc) != len("at")+28 || len(rec.RefreshTokenEnc) != len("rt")+28 {
		t.Fatal("tokens must be stored encrypted")
	}
	if rec.VerifiedAt == nil {
		t.Fatal("verified time missing")
	}

	// The state is single-use.
	if _, err := f.svc.Callback(ctx, "alice", state, "good", ""); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("expected replayed state to be rejected, got %v", err)
	}
}

func TestCallbackRejectsInvalidStates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Callback(ctx, "alice", "unknown", "good", ""); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("unknown state: %v", err)
	}
	authURL, _ := f.svc.Start(ctx, "alice", "ch1")
	f.clock = f.clock.Add(11 * time.Minute)
	if _, err := f.svc.Callback(ctx, "alice", stateFrom(t, authURL), "good", ""); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("expired state: %v", err)
	}
	f.clock = f.clock.Add(-11 * time.Minute)
	authURL, _ = f.svc.Start(ctx, "alice", "ch1")
	if _, err := f.svc.Callback(ctx, "mallory", stateFrom(t, authURL), "good", ""); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("other owner: %v", err)
	}
}

func TestCallbackDenied(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	authURL, _ := f.svc.Start(ctx, "alice", "ch1")
	res, err := f.svc.Callback(ctx, "alice", stateFrom(t, authURL), "", "access_denied")
	if !errors.Is(err, ErrDenied) || res.ChannelID != "ch1" {
		t.Fatalf("expected denied, got %v %+v", err, res)
	}
	rec, _ := f.store.Get(ctx, "alice", "ch1", PlatformYouTube)
	if rec.Status != StatusError {
		t.Fatalf("status = %s", rec.Status)
	}
}

func TestNoChannelAndMissingUploadScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.yt.channels = nil
	authURL, _ := f.svc.Start(ctx, "alice", "ch1")
	res, err := f.svc.Callback(ctx, "alice", stateFrom(t, authURL), "good", "")
	if err != nil || res.Connection.Status != StatusAuthorized || !strings.Contains(res.Connection.StatusReason, "no YouTube channel") {
		t.Fatalf("no channel: %+v %v", res.Connection, err)
	}

	f.yt.channels = []youtube.ChannelInfo{{ID: "UC123", Title: "Quiet Orbit"}}
	f.oauth.tokens.Scopes = []string{youtube.ScopeReadOnly}
	authURL, _ = f.svc.Start(ctx, "alice", "ch1")
	res, err = f.svc.Callback(ctx, "alice", stateFrom(t, authURL), "good", "")
	if err != nil || res.Connection.Status != StatusAuthorized || res.Connection.CanPublish {
		t.Fatalf("missing upload scope must not verify: %+v %v", res.Connection, err)
	}
}

func TestExternalAccountCannotLinkTwoChannels(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u1, _ := f.svc.Start(ctx, "alice", "ch1")
	if _, err := f.svc.Callback(ctx, "alice", stateFrom(t, u1), "good", ""); err != nil {
		t.Fatal(err)
	}
	u2, _ := f.svc.Start(ctx, "alice", "ch2")
	res, err := f.svc.Callback(ctx, "alice", stateFrom(t, u2), "good", "")
	if !errors.Is(err, ErrExternalInUse) || res.Connection.Status != StatusError {
		t.Fatalf("expected external in use, got %v %+v", err, res.Connection)
	}
}

func TestVerifyRefreshesAndHandlesRevocation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u, _ := f.svc.Start(ctx, "alice", "ch1")
	if _, err := f.svc.Callback(ctx, "alice", stateFrom(t, u), "good", ""); err != nil {
		t.Fatal(err)
	}

	f.clock = f.clock.Add(2 * time.Hour) // access token expired
	f.oauth.refreshed = youtube.Token{AccessToken: "at2", RefreshToken: "rt", Expiry: f.clock.Add(time.Hour)}
	v, err := f.svc.Verify(ctx, "alice", "ch1")
	if err != nil || v.Status != StatusVerified {
		t.Fatalf("verify after refresh: %+v %v", v, err)
	}
	if last := f.yt.tokens[len(f.yt.tokens)-1]; last != "at2" {
		t.Fatalf("expected refreshed token to be used, got %q", last)
	}

	f.clock = f.clock.Add(2 * time.Hour)
	f.oauth.refreshErr = youtube.ErrInvalidGrant
	v, err = f.svc.Verify(ctx, "alice", "ch1")
	if err != nil || v.Status != StatusRevoked || v.CanPublish {
		t.Fatalf("expected revoked, got %+v %v", v, err)
	}
	rec, _ := f.store.Get(ctx, "alice", "ch1", PlatformYouTube)
	if rec.RefreshTokenEnc != nil {
		t.Fatal("tokens must be cleared after revocation")
	}
}

func TestDisconnectRevokesAndDeletes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u, _ := f.svc.Start(ctx, "alice", "ch1")
	_, _ = f.svc.Callback(ctx, "alice", stateFrom(t, u), "good", "")
	if err := f.svc.Disconnect(ctx, "alice", "ch1"); err != nil {
		t.Fatal(err)
	}
	if len(f.oauth.revoked) != 1 || f.oauth.revoked[0] != "rt" {
		t.Fatalf("expected refresh token revoked, got %v", f.oauth.revoked)
	}
	if rec, _ := f.store.Get(ctx, "alice", "ch1", PlatformYouTube); rec != nil {
		t.Fatal("connection must be deleted")
	}
	if err := f.svc.Disconnect(ctx, "alice", "ch1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestStartGuards(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Start(ctx, "bob", "ch1"); !errors.Is(err, channel.ErrNotFound) {
		t.Fatalf("other owner: %v", err)
	}
	var ve *channel.ValidationError
	if _, err := f.svc.Start(ctx, "alice", "old"); !errors.As(err, &ve) {
		t.Fatalf("archived: %v", err)
	}
	unconfigured := NewService(f.store, f.oauth, f.yt, fakeChannels{}, nil)
	if _, err := unconfigured.Start(ctx, "alice", "ch1"); !errors.Is(err, youtube.ErrNotConfigured) {
		t.Fatalf("no encryption key: %v", err)
	}
}

func TestAccessToken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.AccessToken(ctx, "alice", "ch1"); !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("no connection: %v", err)
	}
	u, _ := f.svc.Start(ctx, "alice", "ch1")
	if _, err := f.svc.Callback(ctx, "alice", stateFrom(t, u), "good", ""); err != nil {
		t.Fatal(err)
	}
	tok, err := f.svc.AccessToken(ctx, "alice", "ch1")
	if err != nil || tok != "at" {
		t.Fatalf("token = %q %v", tok, err)
	}
	f.clock = f.clock.Add(3 * time.Hour)
	f.oauth.refreshed = youtube.Token{AccessToken: "at3", Expiry: f.clock.Add(time.Hour)}
	if tok, err := f.svc.AccessToken(ctx, "alice", "ch1"); err != nil || tok != "at3" {
		t.Fatalf("refreshed token = %q %v", tok, err)
	}
	if _, err := f.svc.AccessToken(ctx, "bob", "ch1"); !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("other owner: %v", err)
	}
}
