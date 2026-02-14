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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseOptionalBoolQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		field   string
		want    bool
		wantErr bool
	}{
		{name: "blank", raw: "", field: "flag", want: false},
		{name: "true", raw: "true", field: "flag", want: true},
		{name: "false", raw: "false", field: "flag", want: false},
		{name: "invalid", raw: "nope", field: "flag", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseOptionalBoolQuery(tc.raw, tc.field)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOptionalBoolQuery(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("parseOptionalBoolQuery(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestAPIV1TicketAssignedListRejectsInvalidAgentCompletionPending(t *testing.T) {
	t.Parallel()

	app := &application{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ticket/assigned?agent_completion_pending=not-bool", nil)
	req = req.WithContext(context.WithValue(req.Context(), oauthPrincipalKey, oauthPrincipal{UserID: 42}))
	rec := httptest.NewRecorder()

	app.apiV1TicketAssignedList(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid agent_completion_pending") {
		t.Fatalf("expected invalid agent_completion_pending error, got %q", rec.Body.String())
	}
}
