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

import (
	"net/url"
	"testing"
	"time"

	"github.com/ory/fosite"
)

func TestSnapshotRequester_AcceptsGenericRequest(t *testing.T) {
	req := fosite.NewRequest()
	req.SetID("req-1")
	req.RequestedAt = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	req.Client = &fosite.DefaultClient{ID: "client-1"}
	req.Form = url.Values{
		"client_id":    {"client-1"},
		"redirect_uri": {"http://127.0.0.1:8787/callback"},
	}
	req.SetRequestedScopes(fosite.Arguments{"profile:read", "ticket:read"})
	req.GrantScope("profile:read")
	req.SetSession(&fosite.DefaultSession{
		Subject: "3",
		Extra: map[string]interface{}{
			"k": "v",
		},
	})

	snap, err := snapshotRequester(req)
	if err != nil {
		t.Fatalf("snapshotRequester returned error: %v", err)
	}
	if snap.Kind != "request" {
		t.Fatalf("expected kind=request, got %q", snap.Kind)
	}
	if snap.ClientID != "client-1" {
		t.Fatalf("expected client id client-1, got %q", snap.ClientID)
	}
	if len(snap.GrantedScopes) != 1 || snap.GrantedScopes[0] != "profile:read" {
		t.Fatalf("unexpected granted scopes: %#v", snap.GrantedScopes)
	}
}
