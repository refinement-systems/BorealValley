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
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadAgentStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := agentState{
		ServerURL:    "https://example.test",
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Mode:         agentModeTestCounter,
		Model:        "model-a",
		LMStudioURL:  "http://127.0.0.1:1234",
		Token: oauthTokenState{
			AccessToken:  "access",
			RefreshToken: "refresh",
			TokenType:    "Bearer",
			Scope:        agentScope,
			ExpiresAt:    time.Now().UTC().Add(time.Hour),
		},
		Profile: profileState{
			UserID:    7,
			Username:  "agent",
			ActorID:   "https://example.test/users/agent",
			MainKeyID: "https://example.test/users/agent#main-key",
		},
	}

	if err := saveAgentState(path, state); err != nil {
		t.Fatalf("saveAgentState: %v", err)
	}

	loaded, err := loadAgentState(path)
	if err != nil {
		t.Fatalf("loadAgentState: %v", err)
	}
	if loaded.ServerURL != state.ServerURL {
		t.Fatalf("server URL mismatch: got %q want %q", loaded.ServerURL, state.ServerURL)
	}
	if loaded.Token.AccessToken != state.Token.AccessToken {
		t.Fatalf("access token mismatch: got %q want %q", loaded.Token.AccessToken, state.Token.AccessToken)
	}
	if loaded.Profile.ActorID != state.Profile.ActorID {
		t.Fatalf("actor mismatch: got %q want %q", loaded.Profile.ActorID, state.Profile.ActorID)
	}
	if loaded.Mode != state.Mode {
		t.Fatalf("mode mismatch: got %q want %q", loaded.Mode, state.Mode)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Fatalf("expected UpdatedAt to be set")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected mode 0600, got %o", got)
	}
}

func TestResolveStatePathUsesAbsoluteCustomPath(t *testing.T) {
	custom := filepath.Join(".", "test-state.json")
	resolved, err := resolveStatePath(custom)
	if err != nil {
		t.Fatalf("resolveStatePath: %v", err)
	}
	if !filepath.IsAbs(resolved) {
		t.Fatalf("expected absolute path, got %q", resolved)
	}
}

func TestLoadAgentStateDefaultsModeToLMStudio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	raw := []byte(`{
  "server_url": "https://example.test",
  "client_id": "client",
  "client_secret": "secret",
  "redirect_uri": "http://127.0.0.1:8787/callback",
  "token": {
    "access_token": "access",
    "refresh_token": "refresh",
    "expires_at": "2026-03-12T10:00:00Z"
  },
  "profile": {
    "user_id": 7,
    "username": "agent",
    "actor_id": "https://example.test/users/agent",
    "main_key_id": "https://example.test/users/agent#main-key"
  }
}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	state, err := loadAgentState(path)
	if err != nil {
		t.Fatalf("loadAgentState: %v", err)
	}
	if state.Mode != agentModeLMStudio {
		t.Fatalf("expected default mode %q, got %q", agentModeLMStudio, state.Mode)
	}
}

func TestResolveAgentMode(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "default", raw: "", want: agentModeLMStudio},
		{name: "lmstudio", raw: " lmstudio ", want: agentModeLMStudio},
		{name: "test counter", raw: " test-counter ", want: agentModeTestCounter},
		{name: "invalid", raw: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAgentMode(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got mode %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAgentMode: %v", err)
			}
			if got != tt.want {
				t.Fatalf("mode mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}
