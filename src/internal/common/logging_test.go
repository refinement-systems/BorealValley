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
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSlogLevelFromVerbosity(t *testing.T) {
	tests := []struct {
		name      string
		verbosity int
		want      slog.Level
		wantErr   bool
	}{
		{name: "debug", verbosity: 4, want: slog.LevelDebug},
		{name: "info", verbosity: 3, want: slog.LevelInfo},
		{name: "warn", verbosity: 2, want: slog.LevelWarn},
		{name: "error", verbosity: 1, want: slog.LevelError},
		{name: "none", verbosity: 0, want: slog.Level(12)},
		{name: "too low", verbosity: -1, wantErr: true},
		{name: "too high", verbosity: 5, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SlogLevelFromVerbosity(tt.verbosity)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for verbosity=%d", tt.verbosity)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("unexpected level: got=%d want=%d", got, tt.want)
			}
		})
	}
}

func TestNewLoggerIncludesSourceLocation(t *testing.T) {
	var out bytes.Buffer
	logger := newLogger(slog.LevelDebug, &out)

	logger.Info("hello")

	got := out.String()
	if !strings.Contains(got, "source=") {
		t.Fatalf("expected source field in log output, got: %q", got)
	}

	if !strings.Contains(got, "logging_test.go:") {
		t.Fatalf("expected file and line in source field, got: %q", got)
	}
}
