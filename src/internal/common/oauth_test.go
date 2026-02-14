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

func TestValidateOAuthRedirectURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		ok   bool
	}{
		{name: "https", uri: "https://app.example.com/callback", ok: true},
		{name: "localhost http with port", uri: "http://localhost:8787/callback", ok: true},
		{name: "loopback ip with port", uri: "http://127.0.0.1:9000/cb", ok: true},
		{name: "localhost missing port", uri: "http://localhost/callback", ok: false},
		{name: "non-https non-localhost", uri: "http://example.com/callback", ok: false},
	}
	for _, tc := range tests {
		err := ValidateOAuthRedirectURI(tc.uri)
		if tc.ok && err != nil {
			t.Fatalf("%s: expected valid uri, got err %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s: expected invalid uri", tc.name)
		}
	}
}

func TestNormalizeOAuthScopes(t *testing.T) {
	scopes, err := NormalizeOAuthScopes([]string{"repo:read", "ticket:write", "notification:read", "notification:write", "repo:read"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 4 {
		t.Fatalf("expected 4 scopes, got %v", scopes)
	}
	if _, err := NormalizeOAuthScopes([]string{"unknown:scope"}); err == nil {
		t.Fatal("expected unknown scope error")
	}
}
