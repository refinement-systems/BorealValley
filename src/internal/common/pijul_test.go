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
	"context"
	"errors"
	"os/exec"
	"reflect"
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

func TestClonePijulRepoUsesCloneCommand(t *testing.T) {
	t.Parallel()

	var got [][]string
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		got = append(got, append([]string(nil), args...))
		return nil, nil
	}

	if err := clonePijulRepo(context.Background(), runner, "/source/repo", "/dest/repo"); err != nil {
		t.Fatalf("clonePijulRepo: %v", err)
	}

	want := [][]string{{"clone", "/source/repo", "/dest/repo"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command mismatch: got %v want %v", got, want)
	}
}

func TestCommitPijulChangesSkipsRecordWhenOnlyBaselineUntrackedRemain(t *testing.T) {
	t.Parallel()

	var got [][]string
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		got = append(got, append([]string(nil), args...))
		return []byte("On channel: main\nU .ignore\n"), nil
	}

	recorded, err := commitPijulChanges(context.Background(), runner, "/repo", map[string]struct{}{".ignore": {}}, "TCK-1: Fix bug")
	if err != nil {
		t.Fatalf("commitPijulChanges: %v", err)
	}
	if recorded {
		t.Fatalf("expected no record when only baseline untracked files remain")
	}

	want := [][]string{{"status", "--repository", "/repo", "-u", "--no-prompt"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command mismatch: got %v want %v", got, want)
	}
}

func TestCommitPijulChangesAddsNewUntrackedBeforeRecord(t *testing.T) {
	t.Parallel()

	var got [][]string
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		got = append(got, append([]string(nil), args...))
		switch args[0] {
		case "status":
			return []byte("On channel: main\nM tracked.txt\nU .ignore\nU new.txt\n"), nil
		case "add", "record":
			return nil, nil
		default:
			return nil, errors.New("unexpected command")
		}
	}

	recorded, err := commitPijulChanges(context.Background(), runner, "/repo", map[string]struct{}{".ignore": {}}, "TCK-1: Fix bug")
	if err != nil {
		t.Fatalf("commitPijulChanges: %v", err)
	}
	if !recorded {
		t.Fatalf("expected changes to be recorded")
	}

	want := [][]string{
		{"status", "--repository", "/repo", "-u", "--no-prompt"},
		{"add", "--repository", "/repo", "--no-prompt", "new.txt"},
		{"record", "--repository", "/repo", "-a", "-m", "TCK-1: Fix bug", "--no-prompt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command mismatch: got %v want %v", got, want)
	}
}
