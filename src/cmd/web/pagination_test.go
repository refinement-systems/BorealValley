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
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildNotificationNextLink(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/notification?min_id=10&limit=20", nil)
	link := buildNotificationNextLink(req, 44, 20)
	if !strings.Contains(link, `rel="next"`) {
		t.Fatalf("expected rel=next link, got %q", link)
	}
	if !strings.Contains(link, "max_id=44") {
		t.Fatalf("expected max_id in link, got %q", link)
	}
	if strings.Contains(link, "min_id=") {
		t.Fatalf("expected min_id to be removed, got %q", link)
	}
}

func TestParseRequiredPositiveInt64(t *testing.T) {
	if _, err := parseRequiredPositiveInt64("0"); err == nil {
		t.Fatal("expected error for zero")
	}
	if _, err := parseRequiredPositiveInt64("abc"); err == nil {
		t.Fatal("expected error for invalid value")
	}
	if got, err := parseRequiredPositiveInt64("42"); err != nil || got != 42 {
		t.Fatalf("expected 42, got %d err=%v", got, err)
	}
}
