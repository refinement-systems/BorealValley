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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type agentState struct {
	ServerURL    string          `json:"server_url"`
	ClientID     string          `json:"client_id"`
	ClientSecret string          `json:"client_secret"`
	RedirectURI  string          `json:"redirect_uri"`
	Mode         string          `json:"mode,omitempty"`
	Model        string          `json:"model"`
	LMStudioURL  string          `json:"lmstudio_url"`
	Token        oauthTokenState `json:"token"`
	Profile      profileState    `json:"profile"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type oauthTokenState struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type profileState struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	MainKeyID string `json:"main_key_id"`
}

const (
	agentModeLMStudio    = "lmstudio"
	agentModeTestCounter = "test-counter"
)

func resolveStatePath(raw string) (string, error) {
	if strings.TrimSpace(raw) != "" {
		return filepath.Abs(strings.TrimSpace(raw))
	}
	return filepath.Join(common.EnvDirState(), "agent", "state.json"), nil
}

func loadAgentState(path string) (agentState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return agentState{}, errors.New("state path required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return agentState{}, err
	}
	var state agentState
	if err := json.Unmarshal(data, &state); err != nil {
		return agentState{}, fmt.Errorf("decode state: %w", err)
	}
	mode, err := resolveAgentMode(state.Mode)
	if err != nil {
		return agentState{}, err
	}
	state.Mode = mode
	return state, nil
}

func saveAgentState(path string, state agentState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("state path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	mode, err := resolveAgentMode(state.Mode)
	if err != nil {
		return err
	}
	state.Mode = mode
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func resolveAgentMode(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", agentModeLMStudio:
		return agentModeLMStudio, nil
	case agentModeTestCounter:
		return agentModeTestCounter, nil
	default:
		return "", fmt.Errorf("unsupported agent mode %q", raw)
	}
}
