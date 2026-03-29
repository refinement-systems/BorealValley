// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package main

import (
	"testing"
)

func TestCollabModeResolve(t *testing.T) {
	tests := []struct {
		input   string
		want    CollaborationMode
		wantErr bool
	}{
		{"", CollabModeDefault, false},
		{"default", CollabModeDefault, false},
		{"plan", CollabModePlan, false},
		{"bogus", "", true},
	}
	for _, tc := range tests {
		got, err := resolveCollaborationMode(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("resolveCollaborationMode(%q): expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveCollaborationMode(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("resolveCollaborationMode(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
