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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(stderr io.Writer) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}

	licenseText, err := os.ReadFile(filepath.Join(root, "LICENSE.md"))
	if err != nil {
		return err
	}

	licenseBlock, err := licenseCommentBlock(string(licenseText))
	if err != nil {
		return err
	}

	return processTree(root, licenseBlock, stderr)
}

func processTree(root, licenseBlock string, stderr io.Writer) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		updated, skipped, changed, err := applyLicenseToSource(string(contents), licenseBlock)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if skipped {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			if _, err := fmt.Fprintln(stderr, rel); err != nil {
				return err
			}
			return nil
		}
		if !changed {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(updated), info.Mode().Perm())
	})
}

func applyLicenseToSource(src, licenseBlock string) (updated string, skipped, changed bool, err error) {
	if src == "" {
		return "", false, false, errors.New("empty file")
	}
	if strings.HasPrefix(src, licenseBlock) {
		return src, false, false, nil
	}

	idx := skipLeadingBlankLines(src, 0)
	if startsWithBuildTag(src[idx:]) {
		idx = consumeBuildTagBlock(src, idx)
		idx = skipLeadingBlankLines(src, idx)
	}

	if startsWithLineComment(src[idx:]) || startsWithBlockComment(src[idx:]) {
		return src, true, false, nil
	}

	return licenseBlock + src, false, true, nil
}

func licenseCommentBlock(license string) (string, error) {
	normalized := strings.ReplaceAll(license, "\r\n", "\n")
	normalized = strings.TrimRight(normalized, "\n")
	if normalized == "" {
		return "", errors.New("LICENSE.md is empty")
	}

	lines := strings.Split(normalized, "\n")
	var b strings.Builder
	for _, line := range lines {
		if line == "" {
			b.WriteString("//\n")
			continue
		}
		b.WriteString("// ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String(), nil
}

func skipLeadingBlankLines(src string, idx int) int {
	for idx < len(src) {
		switch src[idx] {
		case '\n':
			idx++
		case '\r':
			if idx+1 < len(src) && src[idx+1] == '\n' {
				idx += 2
			} else {
				idx++
			}
		default:
			return idx
		}
	}
	return idx
}

func consumeBuildTagBlock(src string, idx int) int {
	for idx < len(src) {
		lineEnd := strings.IndexByte(src[idx:], '\n')
		var line string
		if lineEnd == -1 {
			line = src[idx:]
			idx = len(src)
		} else {
			line = src[idx : idx+lineEnd]
			idx += lineEnd + 1
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || startsWithBuildTag(trimmed) {
			continue
		}

		return idx - len(line) - lineEndingWidth(src, idx, lineEnd)
	}

	return idx
}

func lineEndingWidth(src string, idx, lineEnd int) int {
	if lineEnd == -1 {
		return 0
	}
	newlinePos := idx - 1
	if newlinePos > 0 && src[newlinePos-1] == '\r' {
		return 2
	}
	return 1
}

func startsWithBuildTag(src string) bool {
	return strings.HasPrefix(src, "//go:build ") || src == "//go:build" ||
		strings.HasPrefix(src, "// +build ") || src == "// +build"
}

func startsWithLineComment(src string) bool {
	return strings.HasPrefix(src, "//")
}

func startsWithBlockComment(src string) bool {
	return strings.HasPrefix(src, "/*")
}
