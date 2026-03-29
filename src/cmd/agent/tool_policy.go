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

// ToolMutability classifies whether a tool modifies state.
type ToolMutability string

const (
	ToolReadOnly ToolMutability = "read-only"
	ToolMutating ToolMutability = "mutating"
)

var toolMutabilityMap = map[string]ToolMutability{
	"list_dir":    ToolReadOnly,
	"read_file":   ToolReadOnly,
	"search_text": ToolReadOnly,
	"write_file":  ToolMutating,
}

// ToolIsMutating reports whether the named tool modifies state.
// Unknown tools return true (conservative default).
func ToolIsMutating(name string) bool {
	m, ok := toolMutabilityMap[name]
	if !ok {
		return true
	}
	return m == ToolMutating
}
