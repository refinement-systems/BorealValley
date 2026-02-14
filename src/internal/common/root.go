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

package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	RootConfigFileName = "config.json"
	RepoDirName        = "repo"

	RepoIDFileName = ".borealvalley-repo-id"
)

// RootConfig is persisted in $ROOT/config.json.
type RootConfig struct {
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
}

func RootPathDefault() string {
	return EnvDirData()
}

func RootConfigPath(root string) string {
	return filepath.Join(root, RootConfigFileName)
}

func RootRepoPath(root string) string {
	return filepath.Join(root, RepoDirName)
}

func InitRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("root path is required")
	}

	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create root directory: %w", err)
	}
	if err := os.MkdirAll(RootRepoPath(root), 0o700); err != nil {
		return fmt.Errorf("create repo directory: %w", err)
	}

	cfgPath := RootConfigPath(root)
	if _, err := os.Stat(cfgPath); err == nil {
		_, err := LoadRootConfig(root)
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat config: %w", err)
	}

	cfg := RootConfig{Hostname: "bv.local", Port: 4000}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(cfgPath, payload, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func LoadRootConfig(root string) (RootConfig, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return RootConfig{}, errors.New("root path is required")
	}

	cfgPath := RootConfigPath(root)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return RootConfig{}, fmt.Errorf("read config %q: %w", cfgPath, err)
	}

	var cfg RootConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return RootConfig{}, fmt.Errorf("parse config %q: %w", cfgPath, err)
	}

	cfg.Hostname = strings.TrimSpace(cfg.Hostname)
	if err := validateConfiguredHostname(cfg.Hostname); err != nil {
		return RootConfig{}, err
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return RootConfig{}, errors.New("config port must be in 1..65535")
	}

	if stat, err := os.Stat(RootRepoPath(root)); err != nil {
		return RootConfig{}, fmt.Errorf("repo directory missing: %w", err)
	} else if !stat.IsDir() {
		return RootConfig{}, errors.New("repo path is not a directory")
	}

	return cfg, nil
}

func CanonicalBaseURL(cfg RootConfig) string {
	host := strings.TrimSpace(cfg.Hostname)
	if hostnameLooksLikeOrigin(host) {
		if origin, err := parseConfiguredOrigin(host); err == nil {
			return origin
		}
	}
	if hostnameHasPort(host) {
		return "https://" + host
	}
	if cfg.Port == 443 {
		return "https://" + host
	}
	return "https://" + host + ":" + strconv.Itoa(cfg.Port)
}

func hostnameHasPort(host string) bool {
	if strings.Count(host, ":") == 0 {
		return false
	}
	if strings.HasPrefix(host, "[") {
		_, _, err := net.SplitHostPort(host)
		return err == nil
	}
	if strings.Count(host, ":") > 1 {
		// Unbracketed IPv6 literals are not host:port.
		return false
	}
	_, _, err := net.SplitHostPort(host)
	return err == nil
}

func validateConfiguredHostname(host string) error {
	if host == "" {
		return errors.New("config hostname is required")
	}
	if hostnameLooksLikeOrigin(host) {
		if _, err := parseConfiguredOrigin(host); err != nil {
			return errors.New("config hostname must be host[:port] or http(s)://host[:port]")
		}
		return nil
	}
	if strings.Contains(host, "/") {
		return errors.New("config hostname must not contain '/'")
	}
	return nil
}

func hostnameLooksLikeOrigin(host string) bool {
	lower := strings.ToLower(host)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func parseConfiguredOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("unsupported scheme")
	}
	if u.Host == "" {
		return "", errors.New("missing host")
	}
	if u.User != nil {
		return "", errors.New("userinfo not supported")
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("path not supported")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("query and fragment not supported")
	}
	return scheme + "://" + u.Host, nil
}
