// Package oauth wraps Google OAuth 2.0 for Gmail integration.
// Provides the auth URL, the code-exchange, and on-demand token refresh.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

// GmailScopes are the scopes we request from Google.
//   - gmail.send / modify / readonly let us send and read messages
//   - openid + email + profile let us fetch the user's email address after auth
var GmailScopes = []string{
	gmail.GmailSendScope,
	gmail.GmailModifyScope,
	gmail.GmailReadonlyScope,
	"openid",
	"email",
	"profile",
}

type GmailOAuth struct {
	cfg *oauth2.Config
}

// NewGmailOAuth builds the OAuth config from app credentials + redirect URL.
func NewGmailOAuth(clientID, clientSecret, redirectURL string) *GmailOAuth {
	return &GmailOAuth{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       GmailScopes,
			Endpoint:     google.Endpoint,
		},
	}
}

// AuthCodeURL returns the URL to send the user to for consent.
// `state` is CSRF protection: generate random, store server-side, verify on callback.
// `loginHint` pre-fills the email if known (optional).
func (g *GmailOAuth) AuthCodeURL(state, loginHint string) string {
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,             // forces a refresh_token on first grant
		oauth2.SetAuthURLParam("prompt", "consent"), // always show consent so refresh_token is always issued
	}
	if loginHint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", loginHint))
	}
	return g.cfg.AuthCodeURL(state, opts...)
}

// Exchange swaps the authorization code from the callback for access/refresh tokens.
func (g *GmailOAuth) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	tok, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oauth: exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		// Shouldn't happen with AccessTypeOffline+prompt=consent, but defend.
		return nil, errors.New("oauth: exchange returned no refresh_token (re-grant required)")
	}
	return tok, nil
}

// UserInfo returned by Google's userinfo endpoint.
type UserInfo struct {
	Email         string `json:"email"`
	Name          string `json:"name"`
	EmailVerified bool   `json:"email_verified"`
	Sub           string `json:"sub"`
}

// FetchUserInfo uses the access token to ask Google who the user is.
// We need at least the email address to label the connected channel.
func (g *GmailOAuth) FetchUserInfo(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	client := g.cfg.Client(ctx, token)
	client.Timeout = 10 * time.Second
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth: userinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth: userinfo status %d", resp.StatusCode)
	}
	var u UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("oauth: decode userinfo: %w", err)
	}
	return &u, nil
}

// TokenSource returns a reusable TokenSource that refreshes automatically
// when the access token is near expiry. Use for long-running goroutines
// (like the polling worker) — each Gmail API call goes through this source.
func (g *GmailOAuth) TokenSource(ctx context.Context, t *oauth2.Token) oauth2.TokenSource {
	return g.cfg.TokenSource(ctx, t)
}
