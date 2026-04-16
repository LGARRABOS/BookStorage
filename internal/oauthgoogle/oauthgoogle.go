package oauthgoogle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bookstorage/internal/config"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const userinfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"

var googleScopes = []string{"openid", "email", "profile"}

// OAuth2Config builds the OAuth2 client config for Google (web redirect flow).
func OAuth2Config(s *config.Settings) *oauth2.Config {
	if s == nil || !s.GoogleOAuthConfigured() {
		return nil
	}
	origin := strings.TrimRight(strings.TrimSpace(s.PublicOrigin), "/")
	return &oauth2.Config{
		ClientID:     s.GoogleClientID,
		ClientSecret: s.GoogleClientSecret,
		RedirectURL:  origin + "/auth/google/callback",
		Scopes:       googleScopes,
		Endpoint:     google.Endpoint,
	}
}

// NewPKCEVerifier returns a PKCE code_verifier (RFC 7636, 43+ chars from unreserved set).
func NewPKCEVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// S256Challenge returns the PKCE code_challenge for the given verifier.
func S256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// AuthCodeURL returns the Google authorization URL with PKCE (S256).
func AuthCodeURL(conf *oauth2.Config, state, codeVerifier string) string {
	return conf.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", S256Challenge(codeVerifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// Exchange exchanges the authorization code for tokens (with PKCE verifier).
func Exchange(ctx context.Context, conf *oauth2.Config, code, codeVerifier string) (*oauth2.Token, error) {
	return conf.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
}

// UserInfo holds stable Google identity fields from userinfo.
type UserInfo struct {
	Sub   string
	Email string
}

// TestUserInfoHook, if non-nil, replaces the real userinfo HTTP call (tests only).
var TestUserInfoHook func(ctx context.Context, accessToken string) (UserInfo, error)

// FetchUserInfo calls Google's userinfo endpoint with the access token.
func FetchUserInfo(ctx context.Context, accessToken string) (UserInfo, error) {
	var out UserInfo
	if TestUserInfoHook != nil {
		return TestUserInfoHook(ctx, accessToken)
	}
	if strings.TrimSpace(accessToken) == "" {
		return out, fmt.Errorf("oauthgoogle: empty access token")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, err
	}
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("oauthgoogle: userinfo status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return out, err
	}
	out.Sub = strings.TrimSpace(parsed.Sub)
	out.Email = strings.TrimSpace(parsed.Email)
	if out.Sub == "" {
		return out, fmt.Errorf("oauthgoogle: missing sub in userinfo")
	}
	return out, nil
}
