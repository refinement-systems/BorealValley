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
	"os/exec"
	"testing"
)

func TestIsPijulRepo(t *testing.T) {
	dir := t.TempDir()

	if IsPijulRepo(dir) {
		t.Fatal("expected plain dir to not be a pijul repo")
	}

	if err := exec.Command("pijul", "init", dir).Run(); err != nil {
		t.Fatalf("pijul init: %v", err)
	}

	if !IsPijulRepo(dir) {
		t.Fatal("expected pijul-init'd dir to be a pijul repo")
	}
}
