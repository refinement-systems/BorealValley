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
	"path/filepath"
	"testing"
)

func TestEnvDirHomeUsesRawHome(t *testing.T) {
	resetEnvDirCaches()

	home := t.TempDir()
	t.Setenv("HOME", home)

	realHome, err := RealPath(home)
	if err != nil {
		t.Fatalf("real path: %v", err)
	}

	got := EnvDirHome()
	if got != realHome {
		t.Fatalf("unexpected home dir: got=%q want=%q", got, realHome)
	}
}

func TestEnvDirDataDefaultUsesHomeLocalShare(t *testing.T) {
	resetEnvDirCaches()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	realHome, err := RealPath(home)
	if err != nil {
		t.Fatalf("real path: %v", err)
	}

	got := EnvDirData()
	want := filepath.Join(realHome, ".local/share", "BorealValley")
	if got != want {
		t.Fatalf("unexpected data dir: got=%q want=%q", got, want)
	}
}

func resetEnvDirCaches() {
	envDirHome = envVal{}
	envDirData = envVal{}
	envDirConfig = envVal{}
	envDirState = envVal{}
}
