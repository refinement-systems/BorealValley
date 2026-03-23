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
)

func TestRepoRootPathResolvesWithinCurrentCheckout(t *testing.T) {
	root, err := repoRootDirFromWD()
	if err != nil {
		t.Fatalf("repoRootDirFromWD: %v", err)
	}
	path, err := repoRootPath("cert/bv.local+3.pem")
	if err != nil {
		t.Fatalf("repoRootPath: %v", err)
	}
	if filepath.Base(path) != "bv.local+3.pem" {
		t.Fatalf("unexpected path %q", path)
	}
	want := filepath.Join(root, "cert", "bv.local+3.pem")
	if path != want {
		t.Fatalf("repoRootPath() = %q, want %q", path, want)
	}
}

func TestRepoRootDirFromWDFindsNearestGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.25.0\n"), 0o600); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("MkdirAll nested: %v", err)
	}
	t.Chdir(nested)

	got, err := repoRootDirFromWD()
	if err != nil {
		t.Fatalf("repoRootDirFromWD: %v", err)
	}
	if got != root {
		t.Fatalf("repoRootDirFromWD() = %q, want %q", got, root)
	}
}
