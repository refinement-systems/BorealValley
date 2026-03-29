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
	"regexp"
	"strings"
)

var proposedPlanRE = regexp.MustCompile(`(?s)<proposed_plan>(.*?)</proposed_plan>`)

// ParseProposedPlans extracts all <proposed_plan>...</proposed_plan> blocks
// from text and returns their trimmed inner content. Returns nil if none found.
func ParseProposedPlans(text string) []string {
	matches := proposedPlanRE.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		if content := strings.TrimSpace(m[1]); content != "" {
			result = append(result, content)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
