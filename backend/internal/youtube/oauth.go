package youtube

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth scopes requested for a channel connection (least privilege).
const (
	ScopeReadOnly  = "https://www.googleapis.com/auth/youtube.readonly"
	ScopeUpload    = "https://www.googleapis.com/auth/youtube.upload"
	ScopeAnalytics = "https://www.googleapis.com/auth/yt-analytics.readonly"
)

// ConnectionScopes are requested when connecting a channel.
var ConnectionScopes = []string{ScopeReadOnly, ScopeUpload, ScopeAnalytics}

// Google OAuth endpoints.
const (
	DefaultAuthURL   = "https://accounts.google.com/o/oauth2/v2/auth"
	DefaultTokenURL  = "https://oauth2.googleapis.com/token"
	DefaultRevokeURL = "https://oauth2.googleapis.com/revoke"
)

// ErrInvalidGrant means the authorization code or refresh token is no
// longer valid (revoked, expired, or already used).
var ErrInvalidGrant = errors.New("authorization is no longer valid")

// OAuthConfig configures the Google OAuth client.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	RevokeURL    string
}

// OAuth implements the authorization code flow with PKCE.
type OAuth struct {
	cfg  OAuthConfig
	http *http.Client
}

// NewOAuth builds an OAuth client. Empty endpoint URLs use Google defaults.
func NewOAuth(httpClient *http.Client, cfg OAuthConfig) *OAuth {
	if cfg.AuthURL == "" {
		cfg.AuthURL = DefaultAuthURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = DefaultTokenURL
	}
	if cfg.RevokeURL == "" {
		cfg.RevokeURL = DefaultRevokeURL
	}
	return &OAuth{cfg: cfg, http: httpClient}
}

// Configured reports whether a client ID, secret, and redirect URL exist.
func (o *OAuth) Configured() bool {
	return o.cfg.ClientID != "" && o.cfg.ClientSecret != "" && o.cfg.RedirectURL != ""
}

// Token is an OAuth token set.
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scopes       []string
}

// HasScope reports whether scope was granted.
func (t Token) HasScope(scope string) bool {
	for _, s := range t.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// RandomToken returns a URL-safe random string with n bytes of entropy.
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// PKCEChallenge returns the S256 code challenge for a verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthCodeURL builds the consent URL. Offline access and forced consent
// make Google return a refresh token.
func (o *OAuth) AuthCodeURL(state, verifier string, scopes []string) string {
	q := url.Values{
		"client_id":              {o.cfg.ClientID},
		"redirect_uri":           {o.cfg.RedirectURL},
		"response_type":          {"code"},
		"scope":                  {strings.Join(scopes, " ")},
		"state":                  {state},
		"access_type":            {"offline"},
		"prompt":                 {"consent select_account"},
		"include_granted_scopes": {"true"},
		"code_challenge":         {PKCEChallenge(verifier)},
		"code_challenge_method":  {"S256"},
	}
	return o.cfg.AuthURL + "?" + q.Encode()
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Exchange trades an authorization code for tokens.
func (o *OAuth) Exchange(ctx context.Context, code, verifier string) (Token, error) {
	return o.tokenRequest(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {o.cfg.RedirectURL},
		"code_verifier": {verifier},
	})
}

// Refresh obtains a new access token. The returned token keeps the given
// refresh token when Google does not rotate it.
func (o *OAuth) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	t, err := o.tokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return Token{}, err
	}
	if t.RefreshToken == "" {
		t.RefreshToken = refreshToken
	}
	return t, nil
}

func (o *OAuth) tokenRequest(ctx context.Context, form url.Values) (Token, error) {
	if !o.Configured() {
		return Token{}, ErrNotConfigured
	}
	form.Set("client_id", o.cfg.ClientID)
	form.Set("client_secret", o.cfg.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := o.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Token{}, ctx.Err()
		}
		return Token{}, fmt.Errorf("%w: token endpoint: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Token{}, fmt.Errorf("%w: read token response: %v", ErrUnavailable, err)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Token{}, fmt.Errorf("%w: decode token response (HTTP %d)", ErrUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		if tr.Error == "invalid_grant" {
			return Token{}, ErrInvalidGrant
		}
		if resp.StatusCode >= 500 {
			return Token{}, fmt.Errorf("%w: token endpoint HTTP %d", ErrUnavailable, resp.StatusCode)
		}
		// Error descriptions never contain token values.
		return Token{}, fmt.Errorf("token request rejected: %s %s", tr.Error, tr.ErrorDescription)
	}
	if tr.AccessToken == "" {
		return Token{}, fmt.Errorf("%w: token response without access token", ErrUnavailable)
	}
	return Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		Scopes:       strings.Fields(tr.Scope),
	}, nil
}

// Revoke revokes a token (revoking a refresh token also revokes its access
// tokens). A token that is already invalid is treated as revoked.
func (o *OAuth) Revoke(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.RevokeURL,
		strings.NewReader(url.Values{"token": {token}}.Encode()))
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: revoke endpoint: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest {
		return nil
	}
	return fmt.Errorf("%w: revoke endpoint HTTP %d", ErrUnavailable, resp.StatusCode)
}
