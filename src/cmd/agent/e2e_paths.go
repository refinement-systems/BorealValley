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
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func repoRootPath(relative string) (string, error) {
	root, err := repoRootDirFromWD()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(relative)), nil
}

func repoRootDirFromWD() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("os.Getwd: %w", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root from %s", wd)
		}
		dir = parent
	}
}

func replaceDatabaseInDSN(rawDSN, dbName string) (string, error) {
	rawDSN = strings.TrimSpace(rawDSN)
	dbName = strings.TrimSpace(dbName)
	if rawDSN == "" {
		return "", fmt.Errorf("dsn is required")
	}
	if dbName == "" {
		return "", fmt.Errorf("database name is required")
	}

	u, err := url.Parse(rawDSN)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "postgres", "postgresql":
	default:
		return "", fmt.Errorf("unsupported postgres dsn scheme %q", u.Scheme)
	}
	u.Path = "/" + dbName
	return u.String(), nil
}
