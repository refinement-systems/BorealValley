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

import "testing"

func TestParseObjectTypeString(t *testing.T) {
	got, err := parseObjectType("Repository")
	if err != nil {
		t.Fatalf("parseObjectType: %v", err)
	}
	if got != "Repository" {
		t.Fatalf("got %q", got)
	}
}

func TestParseObjectTypeArrayChoosesKnownType(t *testing.T) {
	got, err := parseObjectType([]any{"Relationship", "TicketDependency"})
	if err != nil {
		t.Fatalf("parseObjectType: %v", err)
	}
	if got != "TicketDependency" {
		t.Fatalf("got %q", got)
	}
}

func TestParseObjectTypeArrayFallsBackFirstString(t *testing.T) {
	got, err := parseObjectType([]any{"CustomType", "Other"})
	if err != nil {
		t.Fatalf("parseObjectType: %v", err)
	}
	if got != "CustomType" {
		t.Fatalf("got %q", got)
	}
}
