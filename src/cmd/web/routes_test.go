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

	"github.com/alexedwards/scs/v2"
)

func TestRegisterRoutesDoesNotPanic(t *testing.T) {
	mux := http.NewServeMux()
	app := &application{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registerRoutes panicked: %v", r)
		}
	}()

	registerRoutes(mux, app)
}

func TestRegisterRoutesIncludesWebTicketPage(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, &application{})

	req := httptest.NewRequest(http.MethodGet, "/web/ticket", nil)
	_, pattern := mux.Handler(req)
	if pattern == "/" {
		t.Fatalf("expected /web/ticket to be registered, got fallback pattern %q", pattern)
	}
}

func TestRegisterRoutesIncludesWebUserPage(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, &application{})

	req := httptest.NewRequest(http.MethodGet, "/web/user/alice", nil)
	_, pattern := mux.Handler(req)
	if pattern == "/" {
		t.Fatalf("expected /web/user/{name} to be registered, got fallback pattern %q", pattern)
	}
}

func TestRegisterRoutesIncludesCanonicalObjectRoutes(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, &application{})

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/users/alice"},
		{method: http.MethodGet, path: "/repo/repo-a"},
		{method: http.MethodGet, path: "/ticket-tracker/tracker-a"},
		{method: http.MethodGet, path: "/ticket-tracker/tracker-a/ticket/ticket-001"},
		{method: http.MethodGet, path: "/ticket-tracker/tracker-a/ticket/ticket-001/comment/comment-001"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		_, pattern := mux.Handler(req)
		if pattern == "/" {
			t.Fatalf("expected %s %s to be registered, got fallback pattern %q", tc.method, tc.path, pattern)
		}
	}
}

func TestRegisterRoutesIncludesWebLanding(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, &application{})

	req := httptest.NewRequest(http.MethodGet, "/web", nil)
	_, pattern := mux.Handler(req)
	if pattern == "/" {
		t.Fatalf("expected /web to be registered, got fallback pattern %q", pattern)
	}
}

func TestRootRedirectsToWebLoginWhenUnauthenticated(t *testing.T) {
	app := &application{sessionManager: scs.New()}
	mux := http.NewServeMux()
	registerRoutes(mux, app)
	handler := app.sessionManager.LoadAndSave(mux)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 for GET /, got %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/web/login" {
		t.Fatalf("expected redirect to /web/login, got %q", got)
	}
}

func TestLegacyWebPathsReturnNotFound(t *testing.T) {
	app := &application{sessionManager: scs.New()}
	mux := http.NewServeMux()
	registerRoutes(mux, app)
	handler := app.sessionManager.LoadAndSave(mux)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/admin"},
		{method: http.MethodGet, path: "/admin/repo"},
		{method: http.MethodGet, path: "/admin/users/alice"},
		{method: http.MethodGet, path: "/login"},
		{method: http.MethodGet, path: "/logout"},
		{method: http.MethodPost, path: "/logout"},
		{method: http.MethodGet, path: "/home"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected %s %s to return 404, got %d", tc.method, tc.path, rr.Code)
		}
	}
}

func TestWebRouteRedirectsToWebAdmin(t *testing.T) {
	app := &application{sessionManager: scs.New()}
	mux := http.NewServeMux()
	registerRoutes(mux, app)
	handler := app.sessionManager.LoadAndSave(mux)

	req := httptest.NewRequest(http.MethodGet, "/web", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected GET /web to return 303, got %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/web/admin" {
		t.Fatalf("expected GET /web redirect to /web/admin, got %q", got)
	}
}

func TestOAuthRoutesStayRegistered(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, &application{})

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/.well-known/oauth-authorization-server"},
		{method: http.MethodGet, path: "/oauth/authorize"},
		{method: http.MethodPost, path: "/oauth/authorize"},
		{method: http.MethodPost, path: "/oauth/token"},
		{method: http.MethodPost, path: "/oauth/revoke"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		_, pattern := mux.Handler(req)
		if pattern == "/" {
			t.Fatalf("expected %s %s to remain registered, got fallback pattern %q", tc.method, tc.path, pattern)
		}
	}
}

func TestRegisterRoutesIncludesTicketCommentAndMemberMutations(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, &application{})

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/web/ticket-tracker/tracker-a/ticket/ticket-001/comment"},
		{method: http.MethodPost, path: "/web/repo/repo-a/member"},
		{method: http.MethodGet, path: "/api/v1/ticket-tracker/tracker-a/ticket/ticket-001/comment"},
		{method: http.MethodPost, path: "/api/v1/ticket-tracker/tracker-a/ticket/ticket-001/comment"},
		{method: http.MethodPost, path: "/api/v1/ticket-tracker/tracker-a/ticket/ticket-001/update"},
		{method: http.MethodGet, path: "/api/v1/ticket-tracker/tracker-a/ticket/ticket-001/version"},
		{method: http.MethodPost, path: "/api/v1/ticket-tracker/tracker-a/ticket/ticket-001/comment/comment-001/update"},
		{method: http.MethodGet, path: "/api/v1/ticket-tracker/tracker-a/ticket/ticket-001/comment/comment-001/version"},
		{method: http.MethodGet, path: "/api/v1/ticket-tracker/tracker-a/ticket/ticket-001/assignee"},
		{method: http.MethodPost, path: "/api/v1/ticket-tracker/tracker-a/ticket/ticket-001/assignee"},
		{method: http.MethodGet, path: "/api/v1/ticket/assigned"},
		{method: http.MethodGet, path: "/api/v1/notification"},
		{method: http.MethodPost, path: "/api/v1/notification/clear"},
		{method: http.MethodPost, path: "/api/v1/notification/reset"},
		{method: http.MethodPost, path: "/api/v1/notification/123"},
		{method: http.MethodGet, path: "/api/v1/repo/repo-a/member"},
		{method: http.MethodPost, path: "/api/v1/repo/repo-a/member"},
		{method: http.MethodGet, path: "/web/notification"},
		{method: http.MethodPost, path: "/web/notification/clear"},
		{method: http.MethodPost, path: "/web/notification/reset"},
		{method: http.MethodPost, path: "/web/notification/123"},
		{method: http.MethodPost, path: "/web/ticket-tracker/tracker-a/ticket/ticket-001/assignee"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		_, pattern := mux.Handler(req)
		if pattern == "/" {
			t.Fatalf("expected %s %s to be registered, got fallback pattern %q", tc.method, tc.path, pattern)
		}
	}
}
