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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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

func TestHasUsablePijulIdentityReturnsFalseWhenListReportsNoIdentities(t *testing.T) {
	t.Parallel()

	runner := func(_ context.Context, args ...string) ([]byte, error) {
		want := []string{"identity", "list", "--no-prompt"}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("command mismatch: got %v want %v", args, want)
		}
		return []byte("No identities found. Use `pijul identity new` to create one.\n"), nil
	}

	ok, err := hasUsablePijulIdentity(context.Background(), runner)
	if err != nil {
		t.Fatalf("hasUsablePijulIdentity: %v", err)
	}
	if ok {
		t.Fatal("expected no usable identity")
	}
}

func TestHasUsablePijulIdentityReturnsTrueWhenIdentityExists(t *testing.T) {
	t.Parallel()

	runner := func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("borealvalley-agent\n"), nil
	}

	ok, err := hasUsablePijulIdentity(context.Background(), runner)
	if err != nil {
		t.Fatalf("hasUsablePijulIdentity: %v", err)
	}
	if !ok {
		t.Fatal("expected usable identity")
	}
}

func TestIsMissingPijulIdentityErrorMatchesKnownMessages(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		errors.New("It doesn't look like you have any identities configured!"),
		errors.New("Error: Cannot get path of un-named identity"),
	} {
		if !IsMissingPijulIdentityError(err) {
			t.Fatalf("expected missing identity match for %q", err)
		}
	}
	if IsMissingPijulIdentityError(errors.New("some other error")) {
		t.Fatal("did not expect unrelated error to match")
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
		{"add", "--repository", "/repo", "--no-prompt", "/repo/new.txt"},
		{"record", "--repository", "/repo", "-a", "-m", "TCK-1: Fix bug", "--no-prompt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command mismatch: got %v want %v", got, want)
	}
}

func TestCommitPijulChangesAddsNewUntrackedFileWithRealPijul(t *testing.T) {
	if _, err := exec.LookPath("pijul"); err != nil {
		t.Skip("pijul not installed")
	}

	repoPath := t.TempDir()
	if err := exec.Command("pijul", "init", repoPath).Run(); err != nil {
		t.Fatalf("pijul init: %v", err)
	}

	baseline, err := SnapshotUntrackedPaths(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("SnapshotUntrackedPaths: %v", err)
	}

	filePath := filepath.Join(repoPath, "hello.py")
	if err := os.WriteFile(filePath, []byte("print(\"hello, world\")\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	recordCalled := false
	runner := func(ctx context.Context, args ...string) ([]byte, error) {
		switch args[0] {
		case "status", "add":
			return runPijulCommand(ctx, args...)
		case "record":
			recordCalled = true
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected pijul command %q", args[0])
		}
	}

	recorded, err := commitPijulChanges(context.Background(), runner, repoPath, baseline, "TCK-1: Add hello")
	if err != nil {
		t.Fatalf("commitPijulChanges: %v", err)
	}
	if !recorded {
		t.Fatal("expected new file to be recorded")
	}
	if !recordCalled {
		t.Fatal("expected record step to be reached")
	}

	statusOut, err := exec.Command("pijul", "status", "--repository", repoPath, "-u", "--no-prompt").CombinedOutput()
	if err != nil {
		t.Fatalf("pijul status: %v\n%s", err, statusOut)
	}
	if strings.Contains(string(statusOut), "U hello.py") {
		t.Fatalf("expected hello.py to be tracked after add, got status:\n%s", statusOut)
	}
}
