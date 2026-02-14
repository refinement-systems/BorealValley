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
	"net/url"
	"testing"
)

func TestBuildAuthorizeURLForcesFreshLoginByDefault(t *testing.T) {
	authURL, err := buildAuthorizeURL(
		"https://example.test/oauth/authorize",
		"client-123",
		"http://127.0.0.1:8787/callback",
		"state-token",
		"challenge-token",
		false,
	)
	if err != nil {
		t.Fatalf("buildAuthorizeURL: %v", err)
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("url.Parse auth URL: %v", err)
	}

	if got := parsed.Query().Get("prompt"); got != "login" {
		t.Fatalf("expected prompt=login, got %q", got)
	}
}

func TestBuildAuthorizeURLOmitsPromptWhenReusingSession(t *testing.T) {
	authURL, err := buildAuthorizeURL(
		"https://example.test/oauth/authorize",
		"client-123",
		"http://127.0.0.1:8787/callback",
		"state-token",
		"challenge-token",
		true,
	)
	if err != nil {
		t.Fatalf("buildAuthorizeURL: %v", err)
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("url.Parse auth URL: %v", err)
	}

	if got := parsed.Query().Get("prompt"); got != "" {
		t.Fatalf("expected prompt to be omitted, got %q", got)
	}
}
