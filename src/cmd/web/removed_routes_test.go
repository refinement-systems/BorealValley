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
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemovedChatAndModelRoutes(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, &application{})

	for _, path := range []string{
		"/web/model",
		"/web/model/events/abc",
		"/web/repo/repo1/chat",
		"/web/repo/repo1/chat/event/job1",
		"/web/repo/repo1/chat/reset",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		_, pattern := mux.Handler(req)
		if pattern != "/" {
			t.Fatalf("expected removed route %s to fall back to /, got pattern %q", path, pattern)
		}
	}
}
