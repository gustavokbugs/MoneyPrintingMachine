// Package connections links internal channels to external platform
// accounts through official OAuth flows. A connection becomes "verified"
// only after the platform API confirms which account was authorized and
// that the permissions needed to publish were granted.
package connections

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/platforms"
	"contentfactory/internal/secrets"
	"contentfactory/internal/youtube"
)

// PlatformYouTube is the platform identifier for YouTube.
const PlatformYouTube = "youtube"

// Connection statuses.
const (
	StatusPending    = "pending_authorization"
	StatusAuthorized = "authorized"
	StatusVerified   = "verified"
	StatusRevoked    = "revoked"
	StatusError      = "error"
)

var (
	// ErrNotFound means the channel has no connection for the platform.
	ErrNotFound = errors.New("connection not found")
	// ErrStateInvalid means the OAuth state is unknown, used, or expired.
	ErrStateInvalid = errors.New("authorization request is invalid or expired; start the connection again")
	// ErrExternalInUse means the external account is linked to another channel.
	ErrExternalInUse = errors.New("this account is already connected to another channel in this workspace")
	// ErrDenied means the user declined authorization.
	ErrDenied = errors.New("authorization was cancelled or denied")
)

// Record is the stored state of one connection, including encrypted tokens.
type Record struct {
	OwnerID          string
	ChannelID        string
	Platform         string
	Status           string
	ExternalID       string
	ExternalName     string
	CanPublish       bool
	CanReadAnalytics bool
	StatusReason     string
	AccessTokenEnc   []byte
	RefreshTokenEnc  []byte
	TokenExpiresAt   *time.Time
	GrantedScopes    []string
	VerifiedAt       *time.Time
	UpdatedAt        time.Time
}

// View converts a record into the token-free read model.
func (r Record) View() platforms.Connection {
	return platforms.Connection{
		Platform: r.Platform, Status: r.Status, ExternalID: r.ExternalID, ExternalName: r.ExternalName,
		CanPublish: r.CanPublish, CanReadAnalytics: r.CanReadAnalytics, StatusReason: r.StatusReason,
		UpdatedAt: r.UpdatedAt,
	}
}

// State is a pending OAuth authorization.
type State struct {
	StateHash   string
	OwnerID     string
	ChannelID   string
	Platform    string
	VerifierEnc []byte
	ExpiresAt   time.Time
}

// Store persists connections and OAuth states.
type Store interface {
	SaveState(ctx context.Context, s State) error
	// ConsumeState deletes and returns an unexpired state, or ErrStateInvalid.
	ConsumeState(ctx context.Context, stateHash string, now time.Time) (State, error)
	// Get returns nil when no connection exists.
	Get(ctx context.Context, ownerID, channelID, platform string) (*Record, error)
	// Upsert creates or replaces a connection; ErrExternalInUse when the
	// external account is linked to another channel.
	Upsert(ctx context.Context, r *Record) error
	Delete(ctx context.Context, ownerID, channelID, platform string) error
}

// OAuthProvider is the Google OAuth client.
type OAuthProvider interface {
	Configured() bool
	AuthCodeURL(state, verifier string, scopes []string) string
	Exchange(ctx context.Context, code, verifier string) (youtube.Token, error)
	Refresh(ctx context.Context, refreshToken string) (youtube.Token, error)
	Revoke(ctx context.Context, token string) error
}

// ChannelLister lists the YouTube channels of the authorized user.
type ChannelLister interface {
	MyChannels(ctx context.Context, accessToken string) ([]youtube.ChannelInfo, error)
}

// Channels loads internal channels with ownership checks.
type Channels interface {
	Get(ctx context.Context, ownerID, id string) (*channel.Channel, error)
}

// Service manages YouTube connections.
type Service struct {
	store    Store
	oauth    OAuthProvider
	yt       ChannelLister
	channels Channels
	box      *secrets.Box
	now      func() time.Time
	stateTTL time.Duration
}

// NewService builds a Service. box may be nil when no encryption key is
// configured; connecting is then unavailable.
func NewService(store Store, oauth OAuthProvider, yt ChannelLister, channels Channels, box *secrets.Box) *Service {
	return &Service{store: store, oauth: oauth, yt: yt, channels: channels, box: box, now: time.Now, stateTTL: 10 * time.Minute}
}

// Configured reports whether YouTube connections can be made.
func (s *Service) Configured() bool { return s.oauth != nil && s.oauth.Configured() && s.box != nil }

func hashState(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

func tokenContext(channelID, kind string) string {
	return PlatformYouTube + ":" + channelID + ":" + kind
}

// Start begins authorization and returns the Google consent URL.
func (s *Service) Start(ctx context.Context, ownerID, channelID string) (string, error) {
	if !s.Configured() {
		return "", youtube.ErrNotConfigured
	}
	ch, err := s.channels.Get(ctx, ownerID, channelID)
	if err != nil {
		return "", err
	}
	if ch.Status == channel.StatusArchived {
		return "", &channel.ValidationError{Field: "status", Message: "archived channels cannot be connected"}
	}
	state, err := youtube.RandomToken(32)
	if err != nil {
		return "", err
	}
	verifier, err := youtube.RandomToken(48)
	if err != nil {
		return "", err
	}
	h := hashState(state)
	verifierEnc, err := s.box.Seal([]byte(verifier), "oauth_state:"+h)
	if err != nil {
		return "", fmt.Errorf("encrypt verifier: %w", err)
	}
	if err := s.store.SaveState(ctx, State{
		StateHash: h, OwnerID: ownerID, ChannelID: ch.ID, Platform: PlatformYouTube,
		VerifierEnc: verifierEnc, ExpiresAt: s.now().Add(s.stateTTL),
	}); err != nil {
		return "", fmt.Errorf("save oauth state: %w", err)
	}
	existing, err := s.store.Get(ctx, ownerID, ch.ID, PlatformYouTube)
	if err != nil {
		return "", fmt.Errorf("load connection: %w", err)
	}
	if existing == nil {
		if err := s.store.Upsert(ctx, &Record{
			OwnerID: ownerID, ChannelID: ch.ID, Platform: PlatformYouTube, Status: StatusPending,
			StatusReason: "Waiting for Google authorization.",
		}); err != nil {
			return "", fmt.Errorf("create pending connection: %w", err)
		}
	}
	return s.oauth.AuthCodeURL(state, verifier, youtube.ConnectionScopes), nil
}

// CallbackResult tells the caller where the flow ended.
type CallbackResult struct {
	ChannelID  string
	Connection *platforms.Connection
}

// Callback completes authorization. ownerID is the current operator; the
// state must have been created by the same owner.
func (s *Service) Callback(ctx context.Context, ownerID, state, code, oauthError string) (CallbackResult, error) {
	if !s.Configured() {
		return CallbackResult{}, youtube.ErrNotConfigured
	}
	if state == "" {
		return CallbackResult{}, ErrStateInvalid
	}
	h := hashState(state)
	st, err := s.store.ConsumeState(ctx, h, s.now())
	if err != nil {
		return CallbackResult{}, err
	}
	res := CallbackResult{ChannelID: st.ChannelID}
	if st.OwnerID != ownerID || st.Platform != PlatformYouTube {
		return res, ErrStateInvalid
	}
	rec, err := s.recordFor(ctx, ownerID, st.ChannelID)
	if err != nil {
		return res, err
	}
	if oauthError != "" || code == "" {
		if rec.Status != StatusVerified {
			rec.Status, rec.StatusReason = StatusError, ErrDenied.Error()
			_ = s.store.Upsert(ctx, rec)
		}
		return res, ErrDenied
	}
	verifier, err := s.box.Open(st.VerifierEnc, "oauth_state:"+h)
	if err != nil {
		return res, ErrStateInvalid
	}
	tok, err := s.oauth.Exchange(ctx, code, string(verifier))
	if err != nil {
		rec.Status, rec.StatusReason = StatusError, "Google rejected the authorization code. Start the connection again."
		_ = s.store.Upsert(ctx, rec)
		return res, fmt.Errorf("exchange authorization code: %w", err)
	}
	if err := s.storeToken(rec, tok); err != nil {
		return res, err
	}
	view, err := s.verifyWithToken(ctx, rec, tok)
	res.Connection = view
	return res, err
}

// Verify re-checks a connection with the platform, refreshing the access
// token when needed.
func (s *Service) Verify(ctx context.Context, ownerID, channelID string) (*platforms.Connection, error) {
	if !s.Configured() {
		return nil, youtube.ErrNotConfigured
	}
	if _, err := s.channels.Get(ctx, ownerID, channelID); err != nil {
		return nil, err
	}
	rec, err := s.store.Get(ctx, ownerID, channelID, PlatformYouTube)
	if err != nil {
		return nil, fmt.Errorf("load connection: %w", err)
	}
	if rec == nil || len(rec.RefreshTokenEnc) == 0 {
		return nil, ErrNotFound
	}
	tok, err := s.currentToken(ctx, rec)
	if errors.Is(err, youtube.ErrInvalidGrant) {
		s.markRevoked(rec)
		if err := s.store.Upsert(ctx, rec); err != nil {
			return nil, fmt.Errorf("save connection: %w", err)
		}
		v := rec.View()
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return s.verifyWithToken(ctx, rec, tok)
}

// Disconnect revokes Google access and removes the connection.
func (s *Service) Disconnect(ctx context.Context, ownerID, channelID string) error {
	if _, err := s.channels.Get(ctx, ownerID, channelID); err != nil {
		return err
	}
	rec, err := s.store.Get(ctx, ownerID, channelID, PlatformYouTube)
	if err != nil {
		return fmt.Errorf("load connection: %w", err)
	}
	if rec == nil {
		return ErrNotFound
	}
	if len(rec.RefreshTokenEnc) > 0 && s.box != nil && s.oauth != nil {
		refresh, err := s.box.Open(rec.RefreshTokenEnc, tokenContext(channelID, "refresh"))
		if err == nil {
			if err := s.oauth.Revoke(ctx, string(refresh)); err != nil {
				return fmt.Errorf("revoke Google access: %w", err)
			}
		}
	}
	return s.store.Delete(ctx, ownerID, channelID, PlatformYouTube)
}

func (s *Service) recordFor(ctx context.Context, ownerID, channelID string) (*Record, error) {
	rec, err := s.store.Get(ctx, ownerID, channelID, PlatformYouTube)
	if err != nil {
		return nil, fmt.Errorf("load connection: %w", err)
	}
	if rec == nil {
		rec = &Record{OwnerID: ownerID, ChannelID: channelID, Platform: PlatformYouTube, Status: StatusPending}
	}
	return rec, nil
}

func (s *Service) storeToken(rec *Record, tok youtube.Token) error {
	access, err := s.box.Seal([]byte(tok.AccessToken), tokenContext(rec.ChannelID, "access"))
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}
	rec.AccessTokenEnc = access
	if tok.RefreshToken != "" {
		refresh, err := s.box.Seal([]byte(tok.RefreshToken), tokenContext(rec.ChannelID, "refresh"))
		if err != nil {
			return fmt.Errorf("encrypt refresh token: %w", err)
		}
		rec.RefreshTokenEnc = refresh
	}
	exp := tok.Expiry
	rec.TokenExpiresAt = &exp
	if len(tok.Scopes) > 0 {
		rec.GrantedScopes = tok.Scopes
	}
	return nil
}

// currentToken returns a usable access token, refreshing it if it expires
// within two minutes.
func (s *Service) currentToken(ctx context.Context, rec *Record) (youtube.Token, error) {
	tok := youtube.Token{Scopes: rec.GrantedScopes}
	if rec.TokenExpiresAt != nil && s.now().Add(2*time.Minute).Before(*rec.TokenExpiresAt) && len(rec.AccessTokenEnc) > 0 {
		access, err := s.box.Open(rec.AccessTokenEnc, tokenContext(rec.ChannelID, "access"))
		if err == nil {
			tok.AccessToken = string(access)
			tok.Expiry = *rec.TokenExpiresAt
			return tok, nil
		}
	}
	refresh, err := s.box.Open(rec.RefreshTokenEnc, tokenContext(rec.ChannelID, "refresh"))
	if err != nil {
		return youtube.Token{}, fmt.Errorf("decrypt refresh token: %w", err)
	}
	fresh, err := s.oauth.Refresh(ctx, string(refresh))
	if err != nil {
		return youtube.Token{}, err
	}
	if len(fresh.Scopes) == 0 {
		fresh.Scopes = rec.GrantedScopes
	}
	if err := s.storeToken(rec, fresh); err != nil {
		return youtube.Token{}, err
	}
	return fresh, nil
}

func (s *Service) markRevoked(rec *Record) {
	rec.Status = StatusRevoked
	rec.StatusReason = "Google access was revoked or expired. Connect the account again."
	rec.AccessTokenEnc, rec.RefreshTokenEnc, rec.TokenExpiresAt = nil, nil, nil
	rec.CanPublish, rec.CanReadAnalytics = false, false
}

// verifyWithToken confirms the authorized channel and permissions.
func (s *Service) verifyWithToken(ctx context.Context, rec *Record, tok youtube.Token) (*platforms.Connection, error) {
	channels, err := s.yt.MyChannels(ctx, tok.AccessToken)
	if err != nil {
		if errors.Is(err, youtube.ErrUnauthorized) {
			s.markRevoked(rec)
			_ = s.store.Upsert(ctx, rec)
		}
		return nil, fmt.Errorf("list authorized channels: %w", err)
	}
	rec.CanPublish = tok.HasScope(youtube.ScopeUpload)
	rec.CanReadAnalytics = tok.HasScope(youtube.ScopeAnalytics)
	switch {
	case len(channels) == 0:
		rec.Status = StatusAuthorized
		rec.ExternalID, rec.ExternalName = "", ""
		rec.StatusReason = "This Google account has no YouTube channel yet. Create the channel on YouTube, then select Verify."
	case !rec.CanPublish:
		rec.Status = StatusAuthorized
		rec.ExternalID, rec.ExternalName = channels[0].ID, channels[0].Title
		rec.StatusReason = "Upload permission was not granted. Connect again and allow uploads."
	default:
		now := s.now().UTC()
		rec.Status = StatusVerified
		rec.ExternalID, rec.ExternalName = channels[0].ID, channels[0].Title
		rec.StatusReason = ""
		rec.VerifiedAt = &now
	}
	err = s.store.Upsert(ctx, rec)
	if errors.Is(err, ErrExternalInUse) {
		rec.Status = StatusError
		rec.StatusReason = ErrExternalInUse.Error()
		rec.ExternalID, rec.ExternalName = "", ""
		rec.VerifiedAt = nil
		if err := s.store.Upsert(ctx, rec); err != nil {
			return nil, fmt.Errorf("save connection: %w", err)
		}
		v := rec.View()
		return &v, ErrExternalInUse
	}
	if err != nil {
		return nil, fmt.Errorf("save connection: %w", err)
	}
	v := rec.View()
	return &v, nil
}

// ErrNotPublishable means the channel has no verified YouTube connection
// with upload permission.
var ErrNotPublishable = errors.New("connect and verify a YouTube account with upload permission first")

// AccessToken returns a fresh access token for publishing to the channel's
// verified YouTube account, refreshing and persisting it when needed.
func (s *Service) AccessToken(ctx context.Context, ownerID, channelID string) (string, error) {
	if !s.Configured() {
		return "", youtube.ErrNotConfigured
	}
	rec, err := s.store.Get(ctx, ownerID, channelID, PlatformYouTube)
	if err != nil {
		return "", fmt.Errorf("load connection: %w", err)
	}
	if rec == nil || rec.Status != StatusVerified || !rec.CanPublish || len(rec.RefreshTokenEnc) == 0 {
		return "", ErrNotPublishable
	}
	tok, err := s.currentToken(ctx, rec)
	if errors.Is(err, youtube.ErrInvalidGrant) {
		s.markRevoked(rec)
		_ = s.store.Upsert(ctx, rec)
		return "", ErrNotPublishable
	}
	if err != nil {
		return "", err
	}
	if err := s.store.Upsert(ctx, rec); err != nil {
		return "", fmt.Errorf("save refreshed token: %w", err)
	}
	return tok.AccessToken, nil
}
