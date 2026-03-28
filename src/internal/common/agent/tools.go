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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func sandboxed(root, userPath string) (string, error) {
	if filepath.IsAbs(userPath) {
		return "", fmt.Errorf("path %q escapes sandbox", userPath)
	}
	abs := filepath.Clean(filepath.Join(root, userPath))
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes sandbox", userPath)
	}
	return abs, nil
}

type DirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size,omitempty"`
}

func ListDir(root, path string) (string, error) {
	abs, err := sandboxed(root, path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	rows := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		var size int64
		if !e.IsDir() {
			if info, err := e.Info(); err == nil {
				size = info.Size()
			}
		}
		rows = append(rows, DirEntry{Name: e.Name(), IsDir: e.IsDir(), Size: size})
	}
	b, _ := json.Marshal(rows)
	return string(b), nil
}

func ReadFile(root, path string) (string, error) {
	abs, err := sandboxed(root, path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	const maxBytes = 32 * 1024
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	return string(data), nil
}

type SearchMatch struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func WriteFile(root, path, content string) error {
	abs, err := sandboxed(root, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

func SearchText(ctx context.Context, root, path, query string) (string, error) {
	abs, err := sandboxed(root, path)
	if err != nil {
		return "", err
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return "", fmt.Errorf("invalid query: %w", err)
	}
	var matches []SearchMatch
	const (
		maxMatches  = 100
		maxFiles    = 10_000
		maxFileSize = 1 << 20 // 1 MB
	)
	fileCount := 0
	err = filepath.WalkDir(abs, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		fileCount++
		if fileCount > maxFiles {
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > maxFileSize {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				matches = append(matches, SearchMatch{File: rel, Line: i + 1, Text: strings.TrimSpace(line)})
				if len(matches) >= maxMatches {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if matches == nil {
		return "[]", nil
	}
	b, _ := json.Marshal(matches)
	return string(b), nil
}
