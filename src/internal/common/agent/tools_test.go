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

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirAndReadWriteSearch(t *testing.T) {
	root := t.TempDir()
	if err := WriteFile(root, "sub/file.txt", "hello world"); err != nil {
		t.Fatal(err)
	}
	listing, err := ListDir(root, "sub")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listing, "file.txt") {
		t.Fatalf("expected file in listing: %s", listing)
	}
	content, err := ReadFile(root, "sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello world" {
		t.Fatalf("unexpected content: %q", content)
	}
	search, err := SearchText(context.Background(), root, ".", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(search, "file.txt") {
		t.Fatalf("expected file in search: %s", search)
	}
}

func TestSandboxBoundaries(t *testing.T) {
	root := t.TempDir()
	if _, err := ListDir(root, "../../etc"); err == nil {
		t.Fatal("expected traversal error")
	}
	if _, err := ReadFile(root, "../x"); err == nil {
		t.Fatal("expected traversal error")
	}
	if err := WriteFile(root, "../x", "bad"); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestSearchInvalidRegex(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchText(context.Background(), root, ".", "[invalid"); err == nil {
		t.Fatal("expected invalid regex error")
	}
}
