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
	"os"
	"path/filepath"
	"strings"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

const (
	repoPathMapFromEnv = "BV_REPO_PATH_MAP_FROM"
	repoPathMapToEnv   = "BV_REPO_PATH_MAP_TO"
)

type repoPathMapper struct {
	from string
	to   string
}

func loadRepoPathMapperFromEnv() (*repoPathMapper, error) {
	return newRepoPathMapper(os.Getenv(repoPathMapFromEnv), os.Getenv(repoPathMapToEnv))
}

func newRepoPathMapper(from, to string) (*repoPathMapper, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" && to == "" {
		return nil, nil
	}
	if from == "" || to == "" {
		return nil, fmt.Errorf("%s and %s must both be set or both be unset", repoPathMapFromEnv, repoPathMapToEnv)
	}
	return &repoPathMapper{
		from: filepath.Clean(from),
		to:   filepath.Clean(to),
	}, nil
}

func (m *repoPathMapper) Translate(path string) string {
	if m == nil {
		return path
	}
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return path
	}
	if path == m.from {
		return m.to
	}
	prefix := m.from + string(filepath.Separator)
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	suffix := strings.TrimPrefix(path, prefix)
	if suffix == "" {
		return m.to
	}
	return filepath.Join(m.to, suffix)
}

func mapRepositoryForAPI(mapper *repoPathMapper, repo common.Repository) common.Repository {
	repo.Path = mapper.Translate(repo.Path)
	return repo
}

func mapRepositoriesForAPI(mapper *repoPathMapper, repos []common.Repository) []common.Repository {
	if len(repos) == 0 {
		return repos
	}
	mapped := make([]common.Repository, 0, len(repos))
	for _, repo := range repos {
		mapped = append(mapped, mapRepositoryForAPI(mapper, repo))
	}
	return mapped
}
