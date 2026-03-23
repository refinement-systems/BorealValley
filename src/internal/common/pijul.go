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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// IsPijulRepo reports whether dir contains a pijul repository (i.e. has a .pijul entry).
func IsPijulRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".pijul"))
	return err == nil
}

type pijulCommandRunner func(ctx context.Context, args ...string) ([]byte, error)

func ClonePijulRepo(ctx context.Context, source, dest string) error {
	return clonePijulRepo(ctx, runPijulCommand, source, dest)
}

func SnapshotUntrackedPaths(ctx context.Context, repoPath string) (map[string]struct{}, error) {
	return snapshotUntrackedPaths(ctx, runPijulCommand, repoPath)
}

func CommitPijulChanges(ctx context.Context, repoPath string, baselineUntracked map[string]struct{}, message string) (bool, error) {
	return commitPijulChanges(ctx, runPijulCommand, repoPath, baselineUntracked, message)
}

func clonePijulRepo(ctx context.Context, runner pijulCommandRunner, source, dest string) error {
	_, err := runner(ctx, "clone", strings.TrimSpace(source), strings.TrimSpace(dest))
	return err
}

func snapshotUntrackedPaths(ctx context.Context, runner pijulCommandRunner, repoPath string) (map[string]struct{}, error) {
	statuses, err := pijulStatus(ctx, runner, repoPath)
	if err != nil {
		return nil, err
	}
	out := map[string]struct{}{}
	for _, status := range statuses {
		if status.Code == "U" {
			out[status.Path] = struct{}{}
		}
	}
	return out, nil
}

func commitPijulChanges(ctx context.Context, runner pijulCommandRunner, repoPath string, baselineUntracked map[string]struct{}, message string) (bool, error) {
	statuses, err := pijulStatus(ctx, runner, repoPath)
	if err != nil {
		return false, err
	}

	newUntracked := make([]string, 0)
	hasTrackedChanges := false
	for _, status := range statuses {
		switch status.Code {
		case "U":
			if _, ok := baselineUntracked[status.Path]; !ok {
				newUntracked = append(newUntracked, status.Path)
			}
		default:
			hasTrackedChanges = true
		}
	}
	if !hasTrackedChanges && len(newUntracked) == 0 {
		return false, nil
	}

	sort.Strings(newUntracked)
	if len(newUntracked) > 0 {
		args := append([]string{"add", "--repository", strings.TrimSpace(repoPath), "--no-prompt"}, newUntracked...)
		if _, err := runner(ctx, args...); err != nil {
			return false, err
		}
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return false, fmt.Errorf("pijul record message is required")
	}
	if _, err := runner(ctx, "record", "--repository", strings.TrimSpace(repoPath), "-a", "-m", message, "--no-prompt"); err != nil {
		return false, err
	}
	return true, nil
}

type pijulStatusEntry struct {
	Code string
	Path string
}

func pijulStatus(ctx context.Context, runner pijulCommandRunner, repoPath string) ([]pijulStatusEntry, error) {
	output, err := runner(ctx, "status", "--repository", strings.TrimSpace(repoPath), "-u", "--no-prompt")
	if err != nil {
		return nil, err
	}
	return parsePijulStatus(output), nil
}

func parsePijulStatus(output []byte) []pijulStatusEntry {
	lines := strings.Split(string(output), "\n")
	statuses := make([]pijulStatusEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "On channel:") {
			continue
		}
		if len(line) < 3 || line[1] != ' ' {
			continue
		}
		statuses = append(statuses, pijulStatusEntry{
			Code: string(line[0]),
			Path: strings.TrimSpace(line[2:]),
		})
	}
	return statuses
}

func runPijulCommand(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "pijul", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) == 0 {
			return output, err
		}
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
