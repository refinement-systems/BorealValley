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

func TestAuthCodeFromCallbackQueryReturnsCode(t *testing.T) {
	query := url.Values{
		"state": {"state-token"},
		"code":  {"code-token"},
	}

	code, err := authCodeFromCallbackQuery(query, "state-token")
	if err != nil {
		t.Fatalf("authCodeFromCallbackQuery: %v", err)
	}
	if code != "code-token" {
		t.Fatalf("expected code-token, got %q", code)
	}
}

func TestAuthCodeFromCallbackQueryReturnsOAuthErrorDescription(t *testing.T) {
	query := url.Values{
		"state":             {"state-token"},
		"error":             {"access_denied"},
		"error_description": {"at least one scope must be approved"},
	}

	_, err := authCodeFromCallbackQuery(query, "state-token")
	if err == nil {
		t.Fatal("expected oauth error")
	}
	if got := err.Error(); got != "oauth authorization failed: access_denied: at least one scope must be approved" {
		t.Fatalf("unexpected oauth error: %q", got)
	}
}

func TestAuthCodeFromCallbackQueryRejectsStateMismatch(t *testing.T) {
	query := url.Values{
		"state": {"wrong-state"},
		"code":  {"code-token"},
	}

	_, err := authCodeFromCallbackQuery(query, "state-token")
	if err == nil {
		t.Fatal("expected state mismatch")
	}
	if got := err.Error(); got != "oauth state mismatch" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestAuthCodeFromCallbackQueryRejectsMissingCode(t *testing.T) {
	query := url.Values{
		"state": {"state-token"},
	}

	_, err := authCodeFromCallbackQuery(query, "state-token")
	if err == nil {
		t.Fatal("expected missing code error")
	}
	if got := err.Error(); got != "authorization code missing" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestLoopbackCallbackReturnsOAuthErrorBody(t *testing.T) {
	state := "state-token"
	errCh := make(chan error, 1)
	codeCh := make(chan string, 1)
	handler := newAuthCodeCallbackHandler(state, errCh, codeCh)

	req := httptest.NewRequest(http.MethodGet, "/callback?state=state-token&error=access_denied&error_description=at+least+one+scope+must+be+approved", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 response, got %d", rr.Code)
	}
	if got := rr.Body.String(); got == "" || got == "missing code\n" {
		t.Fatalf("expected oauth error body, got %q", got)
	}
	select {
	case err := <-errCh:
		if err == nil || err.Error() != "oauth authorization failed: access_denied: at least one scope must be approved" {
			t.Fatalf("unexpected callback error: %v", err)
		}
	default:
		t.Fatal("expected callback error to be reported")
	}
	select {
	case code := <-codeCh:
		t.Fatalf("unexpected code %q", code)
	default:
	}
}
