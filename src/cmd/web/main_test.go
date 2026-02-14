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
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvalidOptionShowsHelpMessage(t *testing.T) {
	bin := buildBinary(t)

	helpMsg, helpCode := runBinary(t, bin, "-help")
	invalidMsg, invalidCode := runBinary(t, bin, "--does-not-exist")

	if helpMsg != invalidMsg {
		t.Fatalf("expected invalid option message to match -help message\nhelp:\n%s\ninvalid:\n%s", helpMsg, invalidMsg)
	}

	if helpCode != 0 {
		t.Fatalf("expected -help to exit with code 0, got %d", helpCode)
	}

	if invalidCode == 0 {
		t.Fatalf("expected invalid option to exit non-zero")
	}
}

func TestVerbosityOutOfRangeShowsUsageAndFails(t *testing.T) {
	bin := buildBinary(t)

	stderr, code := runBinary(t, bin, "-verbosity", "9")

	if code == 0 {
		t.Fatalf("expected out-of-range verbosity to exit non-zero")
	}

	if !strings.Contains(stderr, "verbosity must be between 0 and 4") {
		t.Fatalf("expected range error in stderr, got:\n%s", stderr)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "borealvalley-web")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	return bin
}

func runBinary(t *testing.T, bin string, args ...string) (string, int) {
	t.Helper()

	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stderr.String(), 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stderr.String(), exitErr.ExitCode()
	}

	t.Fatalf("run failed unexpectedly: %v", err)
	return "", 0
}

func TestResolveTLSFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		certFlag string
		keyFlag  string
		isDev    bool
		existing map[string]bool
		wantCert string
		wantKey  string
		wantUse  bool
	}{
		{
			name:     "explicit cert and key override defaults",
			certFlag: "custom.pem",
			keyFlag:  "custom-key.pem",
			isDev:    false,
			existing: map[string]bool{},
			wantCert: "custom.pem",
			wantKey:  "custom-key.pem",
			wantUse:  true,
		},
		{
			name:     "single explicit flag preserves listen and serve tls behavior",
			certFlag: "custom.pem",
			keyFlag:  "",
			isDev:    false,
			existing: map[string]bool{},
			wantCert: "custom.pem",
			wantKey:  "",
			wantUse:  true,
		},
		{
			name:     "uses local bv cert pair when flags omitted",
			isDev:    true,
			existing: map[string]bool{"cert/bv.local+3.pem": true, "cert/bv.local+3-key.pem": true},
			wantCert: "cert/bv.local+3.pem",
			wantKey:  "cert/bv.local+3-key.pem",
			wantUse:  true,
		},
		{
			name:     "does not enable tls when only one default file exists",
			isDev:    true,
			existing: map[string]bool{"cert/bv.local+3.pem": true},
			wantCert: "",
			wantKey:  "",
			wantUse:  false,
		},
		{
			name:     "does not auto-enable local cert pair in prod mode",
			isDev:    false,
			existing: map[string]bool{"cert/bv.local+3.pem": true, "cert/bv.local+3-key.pem": true},
			wantCert: "",
			wantKey:  "",
			wantUse:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			exists := func(path string) bool {
				return tc.existing[path]
			}

			gotCert, gotKey, gotUse := resolveTLSFiles(tc.certFlag, tc.keyFlag, tc.isDev, exists)
			if gotCert != tc.wantCert || gotKey != tc.wantKey || gotUse != tc.wantUse {
				t.Fatalf("resolveTLSFiles() = (%q, %q, %v), want (%q, %q, %v)",
					gotCert, gotKey, gotUse, tc.wantCert, tc.wantKey, tc.wantUse)
			}
		})
	}
}
