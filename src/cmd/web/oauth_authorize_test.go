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
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/ory/fosite"
)

func TestOAuthAuthorizeGetPromptLoginRedirectsToLoginEvenWithExistingSession(t *testing.T) {
	app := &application{sessionManager: scs.New()}
	handler := app.sessionManager.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.sessionManager.Put(r.Context(), "user_id", int64(42))
		app.oauthAuthorizeGet(w, r)
	}))

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=client-1&prompt=login&state=abc123", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertOAuthAuthorizeLoginRedirect(t, rr, "client-1", "abc123")
}

func TestOAuthAuthorizeGetPromptLoginWithoutSessionStripsPromptFromReturnTo(t *testing.T) {
	app := &application{sessionManager: scs.New()}
	handler := app.sessionManager.LoadAndSave(http.HandlerFunc(app.oauthAuthorizeGet))

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=client-1&prompt=login&state=abc123", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertOAuthAuthorizeLoginRedirect(t, rr, "client-1", "abc123")
}

func assertOAuthAuthorizeLoginRedirect(t *testing.T, rr *httptest.ResponseRecorder, clientID, state string) {
	t.Helper()

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rr.Code)
	}

	loginURL, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("url.Parse login redirect: %v", err)
	}
	if loginURL.Path != "/web/login" {
		t.Fatalf("expected redirect to /web/login, got %q", loginURL.Path)
	}

	returnTo := loginURL.Query().Get("return_to")
	if returnTo == "" {
		t.Fatalf("expected return_to in login redirect")
	}

	authorizeURL, err := url.Parse(returnTo)
	if err != nil {
		t.Fatalf("url.Parse return_to: %v", err)
	}
	if authorizeURL.Path != "/oauth/authorize" {
		t.Fatalf("expected authorize return_to path, got %q", authorizeURL.Path)
	}
	if got := authorizeURL.Query().Get("client_id"); got != clientID {
		t.Fatalf("expected client_id %q in return_to, got %q", clientID, got)
	}
	if got := authorizeURL.Query().Get("state"); got != state {
		t.Fatalf("expected state %q in return_to, got %q", state, got)
	}
	if got := authorizeURL.Query().Get("prompt"); got != "" {
		t.Fatalf("expected prompt to be stripped from return_to, got %q", got)
	}
}

func TestWriteAuthorizeErrorUsesEmptyRequesterWhenNil(t *testing.T) {
	writer := &recordingAuthorizeErrorWriter{}
	recorder := httptest.NewRecorder()
	errBoom := errors.New("boom")

	writeAuthorizeError(context.Background(), recorder, writer, nil, errBoom)

	if writer.calls != 1 {
		t.Fatalf("expected one WriteAuthorizeError call, got %d", writer.calls)
	}
	if writer.requester == nil {
		t.Fatal("expected non-nil authorize requester")
	}
	if writer.err != errBoom {
		t.Fatalf("expected error %v, got %v", errBoom, writer.err)
	}
	request, ok := writer.requester.(*fosite.AuthorizeRequest)
	if !ok {
		t.Fatalf("expected *fosite.AuthorizeRequest, got %T", writer.requester)
	}
	if request.GetRedirectURI() != nil {
		t.Fatalf("expected empty fallback requester, got redirect URI %v", request.GetRedirectURI())
	}
}

func TestWriteAuthorizeErrorPreservesRequester(t *testing.T) {
	writer := &recordingAuthorizeErrorWriter{}
	recorder := httptest.NewRecorder()
	errBoom := errors.New("boom")
	requester := fosite.NewAuthorizeRequest()

	writeAuthorizeError(context.Background(), recorder, writer, requester, errBoom)

	if writer.calls != 1 {
		t.Fatalf("expected one WriteAuthorizeError call, got %d", writer.calls)
	}
	if writer.requester != requester {
		t.Fatalf("expected requester to be preserved")
	}
}

type recordingAuthorizeErrorWriter struct {
	calls     int
	requester fosite.AuthorizeRequester
	err       error
}

func (r *recordingAuthorizeErrorWriter) WriteAuthorizeError(ctx context.Context, rw http.ResponseWriter, ar fosite.AuthorizeRequester, err error) {
	r.calls++
	r.requester = ar
	r.err = err
}
