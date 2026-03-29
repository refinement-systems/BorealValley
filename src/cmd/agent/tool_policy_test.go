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

import "testing"

func TestToolIsMutating_knownTools(t *testing.T) {
	cases := []struct {
		name     string
		mutating bool
	}{
		{"list_dir", false},
		{"read_file", false},
		{"search_text", false},
		{"write_file", true},
	}
	for _, tc := range cases {
		got := ToolIsMutating(tc.name)
		if got != tc.mutating {
			t.Errorf("ToolIsMutating(%q) = %v, want %v", tc.name, got, tc.mutating)
		}
	}
}

func TestToolIsMutating_unknown(t *testing.T) {
	if !ToolIsMutating("unknown_tool") {
		t.Error("ToolIsMutating(unknown) should return true (conservative default)")
	}
}

func TestToolMutabilityMap_coverage(t *testing.T) {
	for _, def := range agentTools() {
		name := def.Function.Name
		if _, ok := toolMutabilityMap[name]; !ok {
			t.Errorf("tool %q has no entry in toolMutabilityMap; add a classification", name)
		}
	}
}
