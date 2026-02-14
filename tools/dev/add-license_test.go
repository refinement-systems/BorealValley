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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyLicenseToSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		files      map[string]string
		wantFiles  map[string]string
		wantStderr string
	}{
		{
			name: "plain package file gets license prepended",
			files: map[string]string{
				"LICENSE.md": "Line one.\nLine two.\n",
				"main.go":    "package main\n\nfunc main() {}\n",
			},
			wantFiles: map[string]string{
				"main.go": "// Line one.\n// Line two.\n\npackage main\n\nfunc main() {}\n",
			},
		},
		{
			name: "build tags stay below inserted license",
			files: map[string]string{
				"LICENSE.md": "Line one.\nLine two.\n",
				"main.go":    "//go:build integration\n// +build integration\n\npackage main\n",
			},
			wantFiles: map[string]string{
				"main.go": "// Line one.\n// Line two.\n\n//go:build integration\n// +build integration\n\npackage main\n",
			},
		},
		{
			name: "existing top comment is skipped and reported",
			files: map[string]string{
				"LICENSE.md": "Line one.\nLine two.\n",
				"main.go":    "// Existing header.\npackage main\n",
			},
			wantFiles: map[string]string{
				"main.go": "// Existing header.\npackage main\n",
			},
			wantStderr: "main.go\n",
		},
		{
			name: "already licensed file is unchanged",
			files: map[string]string{
				"LICENSE.md": "Line one.\nLine two.\n",
				"main.go":    "// Line one.\n// Line two.\n\npackage main\n",
			},
			wantFiles: map[string]string{
				"main.go": "// Line one.\n// Line two.\n\npackage main\n",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeFiles(t, root, tt.files)

			stderr := runScript(t, root)
			if stderr != tt.wantStderr {
				t.Fatalf("stderr mismatch\nwant:\n%s\ngot:\n%s", tt.wantStderr, stderr)
			}

			for path, want := range tt.wantFiles {
				gotBytes, err := os.ReadFile(filepath.Join(root, path))
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				if got := string(gotBytes); got != want {
					t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", path, want, got)
				}
			}
		})
	}
}

func TestProcessTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"LICENSE.md": "Line one.\nLine two.\n",
		"plain.go":   "package main\n",
		"header.go":  "// Existing header.\npackage main\n",
		"note.txt":   "package main\n",
	})

	stderr := runScript(t, root)
	if stderr != "header.go\n" {
		t.Fatalf("stderr mismatch\nwant:\n%s\ngot:\n%s", "header.go\n", stderr)
	}

	plainBytes, err := os.ReadFile(filepath.Join(root, "plain.go"))
	if err != nil {
		t.Fatalf("read plain.go: %v", err)
	}
	if got := string(plainBytes); got != "// Line one.\n// Line two.\n\npackage main\n" {
		t.Fatalf("plain.go mismatch\nwant:\n%s\ngot:\n%s", "// Line one.\n// Line two.\n\npackage main\n", got)
	}

	headerBytes, err := os.ReadFile(filepath.Join(root, "header.go"))
	if err != nil {
		t.Fatalf("read header.go: %v", err)
	}
	if got := string(headerBytes); got != "// Existing header.\npackage main\n" {
		t.Fatalf("header.go mismatch\nwant:\n%s\ngot:\n%s", "// Existing header.\npackage main\n", got)
	}

	noteBytes, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil {
		t.Fatalf("read note.txt: %v", err)
	}
	if got := string(noteBytes); got != "package main\n" {
		t.Fatalf("note.txt mismatch\nwant:\n%s\ngot:\n%s", "package main\n", got)
	}
}

func runScript(t *testing.T, dir string) string {
	t.Helper()

	scriptPath, err := filepath.Abs("add-license.go")
	if err != nil {
		t.Fatalf("abs script path: %v", err)
	}

	cmd := exec.Command("go", "run", scriptPath)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("run script: %v\nstderr:\n%s", err, stderr.String())
	}

	return stderr.String()
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for path, contents := range files {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}
