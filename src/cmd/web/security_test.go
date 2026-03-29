// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

//go:build integration

package main

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

// --- Issue #2: IDOR on ticket tracker assignment ---

func TestIntegrationWebRepoTicketTrackerAssignRequiresAccess(t *testing.T) {
	skipUnlessIntegration(t)
	ts, store, repoSlug := newIntegrationServerWithRepo(t, "idor-web-repo")

	if err := store.CreateUser(context.Background(), "webowner", testPassword); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	ownerID, _, _ := store.VerifyUser(context.Background(), "webowner", testPassword)
	if err := store.CreateUser(context.Background(), "weboutsider", testPassword); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := store.AddRepositoryMemberByUsername(context.Background(), repoSlug, "webowner"); err != nil {
		t.Fatalf("AddRepositoryMemberByUsername: %v", err)
	}

	tracker, err := store.CreateTicketTracker(context.Background(), ownerID, "Tracker Web IDOR", "testing")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}

	// Login as outsider via web.
	outsiderClient := newClient(t)
	loginResp := postForm(t, outsiderClient, ts.URL+"/web/login", url.Values{
		"username": {"weboutsider"},
		"password": {testPassword},
	})
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: expected 303, got %d", loginResp.StatusCode)
	}

	// Outsider tries to assign tracker to repo they don't have access to.
	resp := postForm(t, outsiderClient, ts.URL+"/web/repo/"+repoSlug+"/ticket-tracker", url.Values{
		"action":  {"assign"},
		"tracker": {tracker.Slug},
	})
	readBody(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected outsider to get 403, got %d", resp.StatusCode)
	}

	// Outsider tries to unassign.
	resp2 := postForm(t, outsiderClient, ts.URL+"/web/repo/"+repoSlug+"/ticket-tracker", url.Values{
		"action": {"unassign"},
	})
	readBody(t, resp2)
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("expected outsider unassign to get 403, got %d", resp2.StatusCode)
	}

	// Login as owner and verify they can assign.
	ownerClient := newClient(t)
	loginResp2 := postForm(t, ownerClient, ts.URL+"/web/login", url.Values{
		"username": {"webowner"},
		"password": {testPassword},
	})
	loginResp2.Body.Close()

	resp3 := postForm(t, ownerClient, ts.URL+"/web/repo/"+repoSlug+"/ticket-tracker", url.Values{
		"action":  {"assign"},
		"tracker": {tracker.Slug},
	})
	readBody(t, resp3)
	// Owner should get 303 redirect on success.
	if resp3.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected owner to get 303, got %d", resp3.StatusCode)
	}
}

