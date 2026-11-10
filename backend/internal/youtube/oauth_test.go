package youtube

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOAuthFlow(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		switch {
		case r.URL.Path == "/revoke":
			w.WriteHeader(http.StatusOK)
		case form.Get("refresh_token") == "dead":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		case form.Get("grant_type") == "refresh_token":
			_, _ = w.Write([]byte(`{"access_token":"at2","expires_in":3600,"scope":"` + ScopeReadOnly + `"}`))
		default:
			_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600,"scope":"` + ScopeReadOnly + " " + ScopeUpload + `"}`))
		}
	}))
	defer srv.Close()
	o := NewOAuth(srv.Client(), OAuthConfig{
		ClientID: "cid", ClientSecret: "cs", RedirectURL: "http://127.0.0.1:8080/cb",
		AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", RevokeURL: srv.URL + "/revoke",
	})

	u, err := url.Parse(o.AuthCodeURL("st", "verifier", ConnectionScopes))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge") != PKCEChallenge("verifier") || q.Get("code_challenge_method") != "S256" ||
		q.Get("access_type") != "offline" || q.Get("state") != "st" || !strings.Contains(q.Get("scope"), ScopeUpload) {
		t.Fatalf("unexpected auth URL: %s", u)
	}
	if q.Get("client_secret") != "" {
		t.Fatal("client secret must not appear in the auth URL")
	}

	tok, err := o.Exchange(context.Background(), "code", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("code_verifier") != "verifier" || tok.RefreshToken != "rt" || !tok.HasScope(ScopeUpload) {
		t.Fatalf("exchange: %+v form=%v", tok, form)
	}
	ref, err := o.Refresh(context.Background(), "rt")
	if err != nil || ref.AccessToken != "at2" || ref.RefreshToken != "rt" {
		t.Fatalf("refresh: %+v %v", ref, err)
	}
	if _, err := o.Refresh(context.Background(), "dead"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected invalid grant, got %v", err)
	}
	if err := o.Revoke(context.Background(), "rt"); err != nil {
		t.Fatal(err)
	}
}

func TestPKCEChallengeKnownVector(t *testing.T) {
	// RFC 7636 appendix B.
	if got := PKCEChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge = %s", got)
	}
}

func TestOAuthNotConfigured(t *testing.T) {
	o := NewOAuth(http.DefaultClient, OAuthConfig{})
	if o.Configured() {
		t.Fatal("empty config must not be configured")
	}
	if _, err := o.Exchange(context.Background(), "c", "v"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected not configured, got %v", err)
	}
}
