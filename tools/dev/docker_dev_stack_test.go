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
	"strings"
	"testing"
)

func TestDockerDevStackPropagatesRepoPathMappingEnv(t *testing.T) {
	t.Parallel()

	stackScript, err := os.ReadFile(filepath.Join("..", "deploy", "docker-dev-stack.sh"))
	if err != nil {
		t.Fatalf("read docker-dev-stack.sh: %v", err)
	}
	composeFile, err := os.ReadFile(filepath.Join("..", "deploy", "docker-compose.dev.yml"))
	if err != nil {
		t.Fatalf("read docker-compose.dev.yml: %v", err)
	}

	for _, want := range []string{
		"BV_REPO_PATH_MAP_FROM",
		"BV_REPO_PATH_MAP_TO",
	} {
		if !strings.Contains(string(stackScript), want) {
			t.Fatalf("expected docker-dev-stack.sh to mention %q", want)
		}
		if !strings.Contains(string(composeFile), want) {
			t.Fatalf("expected docker-compose.dev.yml to mention %q", want)
		}
	}
}
