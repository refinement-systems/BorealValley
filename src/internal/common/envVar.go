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
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type envVal struct {
	lock  sync.RWMutex
	value string
}

// RealPath returns the absolute, symlink-resolved path of the given path.
// It fails if any component of the path does not exist.
func RealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Resolves symlinks and cleans the result.
	// Fails if the path (or parts of it) do not exist.
	return filepath.EvalSymlinks(abs)
}

// envOrDefault returns the resolved directory path for the given XDG environment
// variable, falling back to def when the variable is unset. If addAppDir is true,
// "BorealValley" is appended to the path. The directory is created (mode 0700) if it
// does not exist. Results are cached; the function panics on filesystem errors or if a
// conflicting value is ever produced.
func envOrDefault(varname string, cache *envVal, def string, addAppDir bool) string {
	if cache == nil {
		panic("invariant violated: null cache pointer")
	}

	slog.Debug("env read or default", "var", varname, "def", def)

	cache.lock.RLock()
	result := cache.value
	cache.lock.RUnlock()

	if result == "" {
		var envPath = os.Getenv(varname)
		slog.Debug("env read", "var", varname, "val", envPath)

		if envPath == "" {
			envPath = def
			slog.Debug("env: resetting to default", "var", varname, "val", envPath)
		}

		if addAppDir {
			envPath = filepath.Join(envPath, "BorealValley")
		}

		stat, err := os.Stat(envPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// permissions as specified by doc/external/XDG-base-directory.md
				if err := os.MkdirAll(envPath, 0o700); err != nil {
					panic(fmt.Errorf("env %q: fs error: could not create directory %q: %w", varname, envPath, err))
				}
			} else {
				panic(fmt.Errorf("env %q: fs error: could not stat %q: %w", varname, envPath, err))
			}
		} else {
			if !stat.IsDir() {
				panic(fmt.Errorf("env %q: fs error: exists but is not a directory: %q", varname, envPath))
			}
		}

		realPath, err := RealPath(envPath)
		if err != nil {
			panic(fmt.Errorf("env %q: invariant violated: failed to get real path of %q: %w", varname, envPath, err))
		}

		slog.Debug("env: realpath", "var", varname, "val", envPath)

		result = realPath

		cache.lock.Lock()
		if (cache.value != "") && (cache.value != result) {
			panic(fmt.Errorf("env %q: invariant violated: trying to replace %q with %q", varname, cache.value, result))
		}
		cache.value = result
		cache.lock.Unlock()
	}

	return result
}

var envDirHome = envVal{value: ""}

// EnvDirHome returns the user's home directory from $HOME. Panics if $HOME is unset.
func EnvDirHome() string {
	return envOrDefault("HOME", &envDirHome, "", false)
}

var envDirData = envVal{value: ""}

// EnvDirData returns the XDG data directory for BorealValley
// ($XDG_DATA_HOME/BorealValley, defaulting to ~/.local/share/BorealValley).
func EnvDirData() string {
	return envOrDefault("XDG_DATA_HOME", &envDirData, filepath.Join(EnvDirHome(), ".local/share"), true)
}

var envDirConfig = envVal{value: ""}

// EnvDirConfig returns the XDG configuration directory for BorealValley
// ($XDG_CONFIG_HOME/BorealValley, defaulting to ~/.config/BorealValley).
func EnvDirConfig() string {
	return envOrDefault("XDG_CONFIG_HOME", &envDirConfig, filepath.Join(EnvDirHome(), ".config"), true)
}

var envDirState = envVal{value: ""}

// EnvDirState returns the XDG state directory for BorealValley
// ($XDG_STATE_HOME/BorealValley, defaulting to ~/.local/state/BorealValley).
func EnvDirState() string {
	return envOrDefault("XDG_STATE_HOME", &envDirState, filepath.Join(EnvDirHome(), ".local/state"), true)
}
