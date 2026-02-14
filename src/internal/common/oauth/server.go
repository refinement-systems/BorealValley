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

package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type Runtime struct {
	Provider fosite.OAuth2Provider
	Config   *fosite.Config
	Strategy oauth2.CoreStrategy
	Issuer   string
}

func NewRuntime(ctx context.Context, rootDir string, baseURL string, store *common.Store) (*Runtime, error) {
	secret, err := ensureGlobalSecret(rootDir)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("oauth base url required")
	}

	cfg := &fosite.Config{
		AccessTokenLifespan:            time.Hour,
		RefreshTokenLifespan:           30 * 24 * time.Hour,
		AuthorizeCodeLifespan:          10 * time.Minute,
		RefreshTokenScopes:             []string{},
		ScopeStrategy:                  fosite.ExactScopeStrategy,
		EnforcePKCE:                    true,
		EnforcePKCEForPublicClients:    true,
		EnablePKCEPlainChallengeMethod: false,
		TokenURL:                       baseURL + "/oauth/token",
		GlobalSecret:                   secret,
		RedirectSecureChecker:          redirectSecureChecker,
	}

	strategy := compose.NewOAuth2HMACStrategy(cfg)
	provider := compose.Compose(
		cfg,
		store,
		strategy,
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2TokenRevocationFactory,
		compose.OAuth2TokenIntrospectionFactory,
		compose.OAuth2PKCEFactory,
	)

	_ = ctx
	return &Runtime{
		Provider: provider,
		Config:   cfg,
		Strategy: strategy,
		Issuer:   baseURL,
	}, nil
}

func ensureGlobalSecret(rootDir string) ([]byte, error) {
	secretPath := filepath.Join(rootDir, ".oauth-global-secret")
	raw, err := os.ReadFile(secretPath)
	if err == nil {
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, fmt.Errorf("decode oauth global secret: %w", err)
		}
		if len(decoded) < 32 {
			return nil, fmt.Errorf("oauth global secret is too short")
		}
		return decoded, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(buf)
	if err := os.WriteFile(secretPath, []byte(encoded+"\n"), 0o600); err != nil {
		return nil, err
	}
	return buf, nil
}

func redirectSecureChecker(ctx context.Context, u *url.URL) bool {
	if u == nil {
		return false
	}
	hostname := strings.ToLower(strings.TrimSpace(u.Hostname()))
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme == "https" {
		return true
	}
	if scheme != "http" {
		return false
	}
	if u.Port() == "" {
		return false
	}
	if hostname == "localhost" {
		return true
	}
	if ip := net.ParseIP(hostname); ip != nil && ip.IsLoopback() {
		return true
	}
	_ = ctx
	return false
}
