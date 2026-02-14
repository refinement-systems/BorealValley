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
	"fmt"
	"io"
	"log/slog"
	"os"
)

// SlogLevelFromVerbosity maps numeric verbosity to slog levels.
// 4=debug, 3=info, 2=warn, 1=error, 0=silent.
func SlogLevelFromVerbosity(verbosity int) (slog.Level, error) {
	switch verbosity {
	case 4:
		return slog.LevelDebug, nil
	case 3:
		return slog.LevelInfo, nil
	case 2:
		return slog.LevelWarn, nil
	case 1:
		return slog.LevelError, nil
	case 0:
		// Any level above ERROR disables all normal log records.
		return slog.Level(12), nil
	default:
		return slog.LevelInfo, fmt.Errorf("verbosity must be between 0 and 4")
	}
}

// ConfigureLogging installs a text-format slog logger on the default logger at the
// level corresponding to verbosity (0–4). Output goes to stderr.
func ConfigureLogging(verbosity int) error {
	level, err := SlogLevelFromVerbosity(verbosity)
	if err != nil {
		return err
	}

	logger := newLogger(level, os.Stderr)
	slog.SetDefault(logger)
	return nil
}

// newLogger returns a new slog.Logger that writes text-format records to out at the
// given level, with source file and line number included in each record.
func newLogger(level slog.Level, out io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}))
}
