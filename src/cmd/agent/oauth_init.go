// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED “AS IS” AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const agentScope = "profile:read repo:read ticket:read ticket:write"

type initConfig struct {
	ServerURL     string
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	Mode          string
	Model         string
	LMStudioURL   string
	StatePath     string
	NoOpenBrowser bool
	ReuseSession  bool
}

type oauthServerMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func runInit(cfg initConfig) error {
	cfg.ServerURL = strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret = strings.TrimSpace(cfg.ClientSecret)
	cfg.RedirectURI = strings.TrimSpace(cfg.RedirectURI)
	mode, err := resolveAgentMode(cfg.Mode)
	if err != nil {
		return err
	}
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.LMStudioURL = strings.TrimRight(strings.TrimSpace(cfg.LMStudioURL), "/")
	cfg.StatePath = strings.TrimSpace(cfg.StatePath)
	cfg.Mode = mode

	if cfg.ServerURL == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURI == "" {
		return errors.New("server-url, client-id, client-secret, and redirect-uri are required")
	}
	if cfg.Mode == agentModeLMStudio && cfg.Model == "" {
		return errors.New("model is required in lmstudio mode")
	}
	if cfg.Mode == agentModeLMStudio && cfg.LMStudioURL == "" {
		cfg.LMStudioURL = "http://127.0.0.1:1234"
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	meta, err := discoverOAuthMetadata(context.Background(), httpClient, cfg.ServerURL)
	if err != nil {
		return err
	}

	codeVerifier, err := randomToken(48)
	if err != nil {
		return err
	}
	stateToken, err := randomToken(24)
	if err != nil {
		return err
	}
	codeChallenge := pkceChallengeS256(codeVerifier)

	authURL, err := buildAuthorizeURL(meta.AuthorizationEndpoint, cfg.ClientID, cfg.RedirectURI, stateToken, codeChallenge, cfg.ReuseSession)
	if err != nil {
		return err
	}

	fmt.Printf("Open this URL and authorize the app:\n%s\n", authURL)

	code, err := receiveAuthCodeLoopback(authURL, cfg.RedirectURI, stateToken, !cfg.NoOpenBrowser)
	if err != nil {
		slog.Warn("loopback oauth capture failed, falling back to manual input", "err", err)
		code, err = promptForAuthCode(stateToken)
		if err != nil {
			return err
		}
	}

	token, err := exchangeAuthCode(context.Background(), httpClient, meta.TokenEndpoint, cfg.ClientID, cfg.ClientSecret, cfg.RedirectURI, codeVerifier, code)
	if err != nil {
		return err
	}
	profile, err := fetchProfile(context.Background(), httpClient, cfg.ServerURL, token.AccessToken)
	if err != nil {
		return err
	}

	state := agentState{
		ServerURL:    cfg.ServerURL,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURI:  cfg.RedirectURI,
		Mode:         cfg.Mode,
		Model:        cfg.Model,
		LMStudioURL:  cfg.LMStudioURL,
		Token:        token,
		Profile:      profile,
	}
	return saveAgentState(cfg.StatePath, state)
}

func discoverOAuthMetadata(ctx context.Context, client *http.Client, serverURL string) (oauthServerMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/.well-known/oauth-authorization-server", nil)
	if err != nil {
		return oauthServerMetadata{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return oauthServerMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return oauthServerMetadata{}, fmt.Errorf("oauth discovery failed: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var meta oauthServerMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return oauthServerMetadata{}, fmt.Errorf("decode oauth discovery: %w", err)
	}
	meta.AuthorizationEndpoint = strings.TrimSpace(meta.AuthorizationEndpoint)
	meta.TokenEndpoint = strings.TrimSpace(meta.TokenEndpoint)
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return oauthServerMetadata{}, errors.New("oauth discovery missing endpoints")
	}
	return meta, nil
}

func exchangeAuthCode(ctx context.Context, client *http.Client, tokenEndpoint, clientID, clientSecret, redirectURI, codeVerifier, code string) (oauthTokenState, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(code))
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenState{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := client.Do(req)
	if err != nil {
		return oauthTokenState{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return oauthTokenState{}, err
	}
	if resp.StatusCode >= 400 {
		return oauthTokenState{}, fmt.Errorf("oauth token exchange failed: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return oauthTokenState{}, fmt.Errorf("decode oauth token response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return oauthTokenState{}, errors.New("oauth token response missing access_token")
	}
	expiresAt := time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.ExpiresIn <= 0 {
		expiresAt = time.Now().UTC().Add(1 * time.Hour)
	}
	return oauthTokenState{
		AccessToken:  strings.TrimSpace(tokenResp.AccessToken),
		RefreshToken: strings.TrimSpace(tokenResp.RefreshToken),
		TokenType:    strings.TrimSpace(tokenResp.TokenType),
		Scope:        strings.TrimSpace(tokenResp.Scope),
		ExpiresAt:    expiresAt,
	}, nil
}

func refreshAccessToken(ctx context.Context, client *http.Client, tokenEndpoint, clientID, clientSecret string, state oauthTokenState) (oauthTokenState, error) {
	refreshToken := strings.TrimSpace(state.RefreshToken)
	if refreshToken == "" {
		return oauthTokenState{}, errors.New("refresh token missing")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenState{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := client.Do(req)
	if err != nil {
		return oauthTokenState{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return oauthTokenState{}, err
	}
	if resp.StatusCode >= 400 {
		return oauthTokenState{}, fmt.Errorf("oauth refresh failed: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return oauthTokenState{}, fmt.Errorf("decode oauth refresh response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return oauthTokenState{}, errors.New("oauth refresh response missing access_token")
	}
	expiresAt := time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.ExpiresIn <= 0 {
		expiresAt = time.Now().UTC().Add(1 * time.Hour)
	}
	out := oauthTokenState{
		AccessToken: strings.TrimSpace(tokenResp.AccessToken),
		TokenType:   strings.TrimSpace(tokenResp.TokenType),
		Scope:       strings.TrimSpace(tokenResp.Scope),
		ExpiresAt:   expiresAt,
	}
	if strings.TrimSpace(tokenResp.RefreshToken) != "" {
		out.RefreshToken = strings.TrimSpace(tokenResp.RefreshToken)
	} else {
		out.RefreshToken = refreshToken
	}
	return out, nil
}

func buildAuthorizeURL(authEndpoint, clientID, redirectURI, stateToken, codeChallenge string, reuseSession bool) (string, error) {
	u, err := url.Parse(authEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse authorization endpoint: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", agentScope)
	q.Set("state", stateToken)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", codeChallenge)
	if !reuseSession {
		q.Set("prompt", "login")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func receiveAuthCodeLoopback(authorizeURL, redirectURI, expectedState string, openBrowserAutomatically bool) (string, error) {
	redirect, err := url.Parse(redirectURI)
	if err != nil {
		return "", fmt.Errorf("parse redirect uri: %w", err)
	}
	if strings.ToLower(redirect.Scheme) != "http" {
		return "", errors.New("redirect uri must use http for loopback capture")
	}
	host := strings.TrimSpace(redirect.Hostname())
	if host == "" {
		return "", errors.New("redirect uri host required")
	}
	if !(host == "localhost" || isLoopbackIP(host)) {
		return "", errors.New("redirect uri host must be loopback")
	}
	if redirect.Port() == "" {
		return "", errors.New("redirect uri must include explicit port")
	}

	addr := net.JoinHostPort(redirect.Hostname(), redirect.Port())
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	defer ln.Close()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	path := redirect.EscapedPath()
	if path == "" {
		path = "/"
	}

	mux := http.NewServeMux()
	mux.Handle(path, newAuthCodeCallbackHandler(expectedState, errCh, codeCh))

	srv := &http.Server{Handler: mux}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			select {
			case errCh <- serveErr:
			default:
			}
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if openBrowserAutomatically {
		if err := openBrowser(authorizeURL); err != nil {
			slog.Warn("failed to open browser automatically", "err", err)
		}
	}

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-time.After(3 * time.Minute):
		return "", errors.New("timed out waiting for oauth callback")
	}
}

func newAuthCodeCallbackHandler(expectedState string, errCh chan<- error, codeCh chan<- string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, err := authCodeFromCallbackQuery(r.URL.Query(), expectedState)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			select {
			case errCh <- err:
			default:
			}
			return
		}
		_, _ = io.WriteString(w, "<html><body><p>Authorization complete. You can close this window.</p></body></html>")
		select {
		case codeCh <- code:
		default:
		}
	})
}

func promptForAuthCode(expectedState string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Paste redirect URL (or raw code): ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", errors.New("empty oauth input")
	}
	if !strings.Contains(line, "://") {
		return line, nil
	}
	u, err := url.Parse(line)
	if err != nil {
		return "", fmt.Errorf("invalid redirect URL: %w", err)
	}
	return authCodeFromCallbackQuery(u.Query(), expectedState)
}

func authCodeFromCallbackQuery(query url.Values, expectedState string) (string, error) {
	gotState := strings.TrimSpace(query.Get("state"))
	if gotState != expectedState {
		return "", errors.New("oauth state mismatch")
	}
	if oauthErr := strings.TrimSpace(query.Get("error")); oauthErr != "" {
		description := strings.TrimSpace(query.Get("error_description"))
		if description == "" {
			description = strings.TrimSpace(query.Get("error_hint"))
		}
		if description == "" {
			return "", fmt.Errorf("oauth authorization failed: %s", oauthErr)
		}
		return "", fmt.Errorf("oauth authorization failed: %s: %s", oauthErr, description)
	}
	code := strings.TrimSpace(query.Get("code"))
	if code == "" {
		return "", errors.New("authorization code missing")
	}
	return code, nil
}

func pkceChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("token length must be positive")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func openBrowser(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("url required")
	}

	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func isLoopbackIP(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}
