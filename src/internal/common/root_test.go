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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRootCreatesConfigAndRepo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bv-root")
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}

	cfgPath := RootConfigPath(root)
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config stat: %v", err)
	}

	repoPath := RootRepoPath(root)
	if stat, err := os.Stat(repoPath); err != nil {
		t.Fatalf("repo stat: %v", err)
	} else if !stat.IsDir() {
		t.Fatalf("repo path is not directory")
	}

	cfg, err := LoadRootConfig(root)
	if err != nil {
		t.Fatalf("LoadRootConfig: %v", err)
	}
	if cfg.Hostname == "" || cfg.Port == 0 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestCanonicalBaseURL(t *testing.T) {
	if got := CanonicalBaseURL(RootConfig{Hostname: "bv.local", Port: 443}); got != "https://bv.local" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalBaseURL(RootConfig{Hostname: "bv.local", Port: 4000}); got != "https://bv.local:4000" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalBaseURL(RootConfig{Hostname: "example.com:8443", Port: 4000}); got != "https://example.com:8443" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalBaseURL(RootConfig{Hostname: "example.com:8443", Port: 443}); got != "https://example.com:8443" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalBaseURL(RootConfig{Hostname: "https://example.com", Port: 4000}); got != "https://example.com" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalBaseURL(RootConfig{Hostname: "https://example.com:8443", Port: 4000}); got != "https://example.com:8443" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadRootConfigAllowsHTTPSOriginHostname(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bv-root")
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}

	cfgPath := RootConfigPath(root)
	raw := []byte("{\"hostname\":\"https://example.com\",\"port\":4000}\n")
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadRootConfig(root)
	if err != nil {
		t.Fatalf("LoadRootConfig: %v", err)
	}
	if cfg.Hostname != "https://example.com" {
		t.Fatalf("unexpected hostname: %q", cfg.Hostname)
	}
}

func TestLoadRootConfigRejectsHostnameWithPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bv-root")
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}

	cfgPath := RootConfigPath(root)
	raw := []byte("{\"hostname\":\"https://example.com/path\",\"port\":4000}\n")
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadRootConfig(root)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "host[:port]") {
		t.Fatalf("unexpected error: %v", err)
	}
}
