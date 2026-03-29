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

package agent

import (
	"reflect"
	"testing"
)

func TestParseProposedPlans(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "no plan tags",
			input: "Just a normal answer with no tags.",
			want:  nil,
		},
		{
			name:  "single plan",
			input: "Here is my plan:\n<proposed_plan>\nStep 1: do X\nStep 2: do Y\n</proposed_plan>",
			want:  []string{"Step 1: do X\nStep 2: do Y"},
		},
		{
			name:  "multiple plans",
			input: "<proposed_plan>Plan A</proposed_plan> text <proposed_plan>Plan B</proposed_plan>",
			want:  []string{"Plan A", "Plan B"},
		},
		{
			name:  "empty content tags",
			input: "<proposed_plan></proposed_plan>",
			want:  nil,
		},
		{
			name:  "whitespace-only content",
			input: "<proposed_plan>   \n   </proposed_plan>",
			want:  nil,
		},
		{
			name:  "whitespace trimming",
			input: "<proposed_plan>  \n  my plan  \n  </proposed_plan>",
			want:  []string{"my plan"},
		},
		{
			name:  "unclosed tag ignored",
			input: "<proposed_plan>no closing tag",
			want:  nil,
		},
		{
			name:  "wrong tag name",
			input: "<proposed_plans>wrong tag name</proposed_plans>",
			want:  nil,
		},
		{
			name:  "valid plan mixed with malformed",
			input: "<proposed_plan>good plan</proposed_plan><proposed_plans>bad</proposed_plans>",
			want:  []string{"good plan"},
		},
		{
			name:  "multiline plan content",
			input: "<proposed_plan>\n1. Edit foo.go\n2. Edit bar.go\n</proposed_plan>",
			want:  []string{"1. Edit foo.go\n2. Edit bar.go"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseProposedPlans(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseProposedPlans(%q)\n  got:  %v\n  want: %v", tc.input, got, tc.want)
			}
		})
	}
}
