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

//go:build integration

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

const testPassword = "test-password-long"

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("set RUN_INTEGRATION=1 to run integration tests")
	}
}

func integrationPostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("BV_TEST_PG_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv(common.PostgresDSNEnv))
	}
	if dsn == "" {
		t.Skip("set BV_TEST_PG_DSN (or BV_PG_DSN) to run integration tests")
	}
	return dsn
}

// newIntegrationServer starts an in-process HTTP server using a temp root
// directory and PostgreSQL-backed store.
func newIntegrationServer(t *testing.T) (*httptest.Server, *common.Store) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	if err := common.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	handler, store, err := newHandler(root, integrationPostgresDSN(t), true /* dev */)
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	ts := httptest.NewServer(handler)
	t.Cleanup(func() {
		ts.Close()
		store.Close()
	})
	return ts, store
}

func newIntegrationServerWithRepo(t *testing.T, repoName string) (*httptest.Server, *common.Store, string) {
	t.Helper()

	root := filepath.Join(t.TempDir(), "root")
	if err := common.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}

	repoDir := filepath.Join(common.RootRepoPath(root), repoName)
	if err := os.MkdirAll(repoDir, 0o700); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	if out, err := exec.Command("git", "-C", repoDir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	handler, store, err := newHandler(root, integrationPostgresDSN(t), true /* dev */)
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	ts := httptest.NewServer(handler)
	t.Cleanup(func() {
		ts.Close()
		store.Close()
	})

	repo, found, err := store.GetRepositoryBySlug(context.Background(), repoName)
	if err != nil {
		t.Fatalf("GetRepositoryBySlug: %v", err)
	}
	if !found {
		t.Fatalf("repository %q not found after startup resync", repoName)
	}
	return ts, store, repo.Slug
}

// newClient returns an http.Client with a cookie jar that does not follow
// redirects automatically, so tests can inspect redirect responses directly.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// postForm sends a POST with form-encoded vals to target. It sets the Origin
// header to the target's scheme+host so the CSRF middleware passes in dev mode.
func postForm(t *testing.T, client *http.Client, target string, vals url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(vals.Encode()))
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	u, _ := url.Parse(target)
	req.Header.Set("Origin", u.Scheme+"://"+u.Host)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do POST %s: %v", target, err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	return string(b)
}

func TestIntegrationUnauthenticatedRedirectsToLogin(t *testing.T) {
	skipUnlessIntegration(t)
	ts, _ := newIntegrationServer(t)
	client := newClient(t)

	resp, err := client.Get(ts.URL + "/web/admin")
	if err != nil {
		t.Fatalf("GET /web/admin: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "/web/login") {
		t.Errorf("expected redirect to /web/login, got %q", loc)
	}
}

func TestIntegrationLoginPageRenders(t *testing.T) {
	skipUnlessIntegration(t)
	ts, _ := newIntegrationServer(t)
	client := newClient(t)

	resp, err := client.Get(ts.URL + "/web/login")
	if err != nil {
		t.Fatalf("GET /web/login: %v", err)
	}
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "<h1>Login</h1>") {
		t.Errorf("expected login form in body, got:\n%s", body)
	}
}

func TestIntegrationBadCredentialsShowsError(t *testing.T) {
	skipUnlessIntegration(t)
	ts, cp := newIntegrationServer(t)
	if err := cp.CreateUser(context.Background(), "testuser", testPassword); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	client := newClient(t)

	resp := postForm(t, client, ts.URL+"/web/login", url.Values{
		"username": {"testuser"},
		"password": {"wrong-password"},
	})
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Invalid credentials") {
		t.Errorf("expected 'Invalid credentials' in body, got:\n%s", body)
	}
}

func TestIntegrationGoodCredentialsRedirectsToHome(t *testing.T) {
	skipUnlessIntegration(t)
	ts, cp := newIntegrationServer(t)
	if err := cp.CreateUser(context.Background(), "testuser", testPassword); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	client := newClient(t)

	resp := postForm(t, client, ts.URL+"/web/login", url.Values{
		"username": {"testuser"},
		"password": {testPassword},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/web/admin" {
		t.Errorf("expected redirect to /web/admin, got %q", loc)
	}
}

func TestIntegrationAuthenticatedCanAccessHome(t *testing.T) {
	skipUnlessIntegration(t)
	ts, cp := newIntegrationServer(t)
	if err := cp.CreateUser(context.Background(), "testuser", testPassword); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	client := newClient(t)

	// Login; the cookie jar captures the session cookie automatically.
	loginResp := postForm(t, client, ts.URL+"/web/login", url.Values{
		"username": {"testuser"},
		"password": {testPassword},
	})
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: expected 303, got %d", loginResp.StatusCode)
	}

	// Access web admin home with the session cookie.
	resp, err := client.Get(ts.URL + "/web/admin")
	if err != nil {
		t.Fatalf("GET /web/admin: %v", err)
	}
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Logged in") {
		t.Errorf("expected 'Logged in' in body, got:\n%s", body)
	}
	if !strings.Contains(body, `href="/web/repo"`) {
		t.Errorf("expected home to link to /web/repo, got:\n%s", body)
	}
	if !strings.Contains(body, `href="/web/ticket-tracker"`) {
		t.Errorf("expected home to link to /web/ticket-tracker, got:\n%s", body)
	}
	if !strings.Contains(body, `href="/web/notification"`) {
		t.Errorf("expected home to link to /web/notification, got:\n%s", body)
	}
	if !strings.Contains(body, `href="/web/oauth/grant"`) {
		t.Errorf("expected home to link to /web/oauth/grant, got:\n%s", body)
	}
}

func TestIntegrationCanonicalUserNegotiatesActivityPubAndHTML(t *testing.T) {
	skipUnlessIntegration(t)
	ts, cp := newIntegrationServer(t)
	if err := cp.CreateUser(context.Background(), "alice", testPassword); err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	if err := cp.CreateUser(context.Background(), "bob", testPassword); err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	client := newClient(t)

	loginResp := postForm(t, client, ts.URL+"/web/login", url.Values{
		"username": {"bob"},
		"password": {testPassword},
	})
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: expected 303, got %d", loginResp.StatusCode)
	}

	apReq, err := http.NewRequest(http.MethodGet, ts.URL+"/users/alice", nil)
	if err != nil {
		t.Fatalf("http.NewRequest AP: %v", err)
	}
	apReq.Header.Set("Accept", "application/activity+json")
	apResp, err := client.Do(apReq)
	if err != nil {
		t.Fatalf("GET canonical user AP: %v", err)
	}
	apBody := readBody(t, apResp)
	if apResp.StatusCode != http.StatusOK {
		t.Fatalf("expected AP GET to return 200, got %d", apResp.StatusCode)
	}
	if got := apResp.Header.Get("Content-Type"); got != apMediaType {
		t.Fatalf("expected AP content type %q, got %q", apMediaType, got)
	}
	if !strings.Contains(apBody, `"type":"Person"`) {
		t.Fatalf("expected AP response to include Person object, got:\n%s", apBody)
	}

	htmlReq, err := http.NewRequest(http.MethodGet, ts.URL+"/users/alice", nil)
	if err != nil {
		t.Fatalf("http.NewRequest HTML: %v", err)
	}
	htmlReq.Header.Set("Accept", "text/html")
	htmlResp, err := client.Do(htmlReq)
	if err != nil {
		t.Fatalf("GET canonical user HTML: %v", err)
	}
	htmlBody := readBody(t, htmlResp)
	if htmlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTML GET to return 200, got %d", htmlResp.StatusCode)
	}
	if got := htmlResp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected HTML content type, got %q", got)
	}
	if !strings.Contains(htmlBody, "<h1>User Actor</h1>") {
		t.Fatalf("expected canonical user HTML page, got:\n%s", htmlBody)
	}
}

func TestIntegrationCanonicalRepoTrackerTicketNegotiation(t *testing.T) {
	skipUnlessIntegration(t)
	ts, cp, repoSlug := newIntegrationServerWithRepo(t, "repo-canonical")
	if err := cp.CreateUser(context.Background(), "ticket-user", testPassword); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	userID, ok, err := cp.VerifyUser(context.Background(), "ticket-user", testPassword)
	if err != nil {
		t.Fatalf("VerifyUser: %v", err)
	}
	if !ok {
		t.Fatal("expected VerifyUser success")
	}

	tracker, err := cp.CreateTicketTracker(context.Background(), userID, "Tracker Canonical", "Canonical testing")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := cp.AssignTicketTrackerToRepository(context.Background(), repoSlug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}
	ticket, err := cp.CreateTicket(context.Background(), userID, tracker.Slug, repoSlug, "Canonical ticket", "Canonical content")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	client := newClient(t)
	loginResp := postForm(t, client, ts.URL+"/web/login", url.Values{
		"username": {"ticket-user"},
		"password": {testPassword},
	})
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: expected 303, got %d", loginResp.StatusCode)
	}

	apReq, err := http.NewRequest(http.MethodGet, ts.URL+"/ticket-tracker/"+tracker.Slug+"/ticket/"+ticket.Slug, nil)
	if err != nil {
		t.Fatalf("http.NewRequest AP: %v", err)
	}
	apReq.Header.Set("Accept", "application/activity+json")
	apResp, err := client.Do(apReq)
	if err != nil {
		t.Fatalf("GET canonical ticket AP: %v", err)
	}
	apBody := readBody(t, apResp)
	if apResp.StatusCode != http.StatusOK {
		t.Fatalf("expected AP ticket GET to return 200, got %d", apResp.StatusCode)
	}
	if got := apResp.Header.Get("Content-Type"); got != apMediaType {
		t.Fatalf("expected AP content type %q, got %q", apMediaType, got)
	}
	if !strings.Contains(apBody, `"type":"Ticket"`) {
		t.Fatalf("expected AP ticket object, got:\n%s", apBody)
	}

	htmlResp, err := client.Get(ts.URL + "/ticket-tracker/" + tracker.Slug + "/ticket/" + ticket.Slug)
	if err != nil {
		t.Fatalf("GET canonical ticket HTML: %v", err)
	}
	htmlBody := readBody(t, htmlResp)
	if htmlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTML ticket GET to return 200, got %d", htmlResp.StatusCode)
	}
	if got := htmlResp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected HTML content type, got %q", got)
	}
	if !strings.Contains(htmlBody, "Ticket Object "+ticket.Slug) {
		t.Fatalf("expected canonical ticket HTML page, got:\n%s", htmlBody)
	}

	missingResp, err := client.Get(ts.URL + "/ticket-tracker/" + tracker.Slug + "/ticket/missing-ticket")
	if err != nil {
		t.Fatalf("GET missing canonical ticket: %v", err)
	}
	readBody(t, missingResp)
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing ticket to return 404, got %d", missingResp.StatusCode)
	}
}
