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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type fakeBVServer struct {
	mu sync.Mutex

	assigned []common.AssignedTicket
	repos    map[string]common.Repository
	comments []fakeTicketComment
	updates  []fakeTicketCommentUpdate
	events   []string

	assignedAuthHeader string
	assignedQuery      string
	repoDetailAuth     string
	repoDetailSlugs    []string

	tokenResponse  oauthTokenResponse
	profilePayload profileState
}

type fakeTicketComment struct {
	Slug             string
	Content          string
	AgentCommentKind string
}

type fakeTicketCommentUpdate struct {
	CommentSlug string
	Content     string
}

func (f *fakeBVServer) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/.well-known/oauth-authorization-server":
		writeJSONResponse(w, http.StatusOK, map[string]any{
			"authorization_endpoint": "http://127.0.0.1/unused",
			"token_endpoint":         serverBaseURL(r) + "/oauth/token",
		})
		return
	case r.Method == http.MethodPost && r.URL.Path == "/oauth/token":
		writeJSONResponse(w, http.StatusOK, f.tokenResponse)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/profile":
		writeJSONResponse(w, http.StatusOK, map[string]any{
			"id":          f.profilePayload.UserID,
			"username":    f.profilePayload.Username,
			"actor_id":    f.profilePayload.ActorID,
			"main_key_id": f.profilePayload.MainKeyID,
		})
		return
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ticket/assigned":
		f.mu.Lock()
		f.assignedAuthHeader = r.Header.Get("Authorization")
		f.assignedQuery = r.URL.RawQuery
		f.mu.Unlock()
		writeJSONResponse(w, http.StatusOK, map[string]any{"ticket": f.assigned})
		return
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/repo/"):
		repoSlug := strings.TrimPrefix(r.URL.Path, "/api/v1/repo/")
		f.mu.Lock()
		f.repoDetailAuth = r.Header.Get("Authorization")
		f.repoDetailSlugs = append(f.repoDetailSlugs, repoSlug)
		repo, ok := f.repos[repoSlug]
		f.mu.Unlock()
		if !ok {
			repo = common.Repository{Slug: repoSlug, Path: "/translated/" + repoSlug}
		}
		writeJSONResponse(w, http.StatusOK, repo)
		return
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/ticket-tracker/") && strings.HasSuffix(r.URL.Path, "/comment"):
		var body struct {
			Content          string `json:"content"`
			AgentCommentKind string `json:"agent_comment_kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		slug := fmt.Sprintf("comment-%d", len(f.comments)+1)
		comment := fakeTicketComment{
			Slug:             slug,
			Content:          body.Content,
			AgentCommentKind: body.AgentCommentKind,
		}
		f.mu.Lock()
		f.comments = append(f.comments, comment)
		f.events = append(f.events, "comment")
		f.mu.Unlock()
		writeJSONResponse(w, http.StatusCreated, common.TicketComment{
			Slug:    slug,
			ActorID: "https://example.test/comment/" + slug,
			Content: body.Content,
		})
		return
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/ticket-tracker/") && strings.Contains(r.URL.Path, "/comment/") && strings.HasSuffix(r.URL.Path, "/update"):
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		commentSlug := ""
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "comment" {
				commentSlug = parts[i+1]
				break
			}
		}
		if commentSlug == "" {
			http.Error(w, "missing comment slug", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.updates = append(f.updates, fakeTicketCommentUpdate{CommentSlug: commentSlug, Content: body.Content})
		f.events = append(f.events, "comment_update")
		f.mu.Unlock()
		writeJSONResponse(w, http.StatusCreated, map[string]any{"status": "ok"})
		return
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/ticket-tracker/") && strings.HasSuffix(r.URL.Path, "/update"):
		http.Error(w, "unexpected ticket update", http.StatusGone)
		return
	default:
		http.NotFound(w, r)
	}
}

func joinCommentUpdateContents(updates []fakeTicketCommentUpdate, commentSlug string) string {
	var values []string
	for _, item := range updates {
		if item.CommentSlug == commentSlug {
			values = append(values, item.Content)
		}
	}
	return strings.Join(values, "\n")
}

func serverBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

type fakeLMStudioServer struct {
	handler func(r *http.Request) (int, any)
}

func (f fakeLMStudioServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	status, payload := f.handler(r)
	writeJSONResponse(w, status, payload)
}

func writeJSONResponse(w http.ResponseWriter, status int, body any) {
	raw, _ := json.Marshal(body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeAgentStateFile(t *testing.T, state agentState) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := saveAgentState(path, state); err != nil {
		t.Fatalf("saveAgentState: %v", err)
	}
	return path
}

func newHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listener unavailable in this environment: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func stubTicketWorkspaceLifecycle(t *testing.T, files map[string]string) {
	t.Helper()

	origPrepare := prepareTicketWorkspaceForRun
	origFinalize := finalizeTicketWorkspaceForRun
	t.Cleanup(func() {
		prepareTicketWorkspaceForRun = origPrepare
		finalizeTicketWorkspaceForRun = origFinalize
	})

	prepareTicketWorkspaceForRun = func(parent string, ticket common.AssignedTicket, repo common.Repository) (ticketWorkspace, error) {
		path := filepath.Join(parent, ticket.RepositorySlug, ticket.TicketSlug)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return ticketWorkspace{}, err
		}
		for name, contents := range files {
			fullPath := filepath.Join(path, name)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return ticketWorkspace{}, err
			}
			if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
				return ticketWorkspace{}, err
			}
		}
		return ticketWorkspace{
			Path:              path,
			SourceRepoPath:    repo.Path,
			BaselineUntracked: map[string]struct{}{},
		}, nil
	}
	finalizeTicketWorkspaceForRun = func(string, ticketWorkspace, common.AssignedTicket) error {
		return nil
	}
}

func TestRunAgentOnceNoEligibleTickets(t *testing.T) {
	bv := &fakeBVServer{}
	bv.tokenResponse = oauthTokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 3600}
	bv.profilePayload = profileState{UserID: 1, Username: "agent", ActorID: "https://example.test/users/agent", MainKeyID: "https://example.test/users/agent#main-key"}
	bvServer := newHTTPServer(t, http.HandlerFunc(bv.handler))

	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{handler: func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "unused"},
				"finish_reason": "stop",
			}},
		}
	}}.serveHTTP))

	statePath := writeAgentStateFile(t, agentState{
		ServerURL:    bvServer.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Model:        "m",
		LMStudioURL:  lm.URL,
		Token: oauthTokenState{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().UTC().Add(30 * time.Minute),
		},
		Profile: profileState{UserID: 1, Username: "agent", ActorID: "https://example.test/users/agent", MainKeyID: "https://example.test/users/agent#main-key"},
	})

	if err := runAgentOnce(runConfig{StatePath: statePath, Workspace: t.TempDir(), MaxIter: 3}); err != nil {
		t.Fatalf("runAgentOnce: %v", err)
	}

	bv.mu.Lock()
	defer bv.mu.Unlock()
	if len(bv.comments) != 0 {
		t.Fatalf("expected no comments, got %d", len(bv.comments))
	}
	if len(bv.updates) != 0 {
		t.Fatalf("expected no updates, got %d", len(bv.updates))
	}
}

func TestRunAgentOnceAcknowledgesThenPublishesUpdates(t *testing.T) {
	workspace := t.TempDir()
	stubTicketWorkspaceLifecycle(t, map[string]string{"README.txt": "hello\n"})

	bv := &fakeBVServer{
		assigned: []common.AssignedTicket{{
			ActorID:        "https://example.test/ticket-tracker/tracker-1/ticket/TCK-1",
			TrackerSlug:    "tracker-1",
			TicketSlug:     "TCK-1",
			RepositorySlug: "repo-1",
			Summary:        "Fix bug",
			Content:        "details",
			CreatedAt:      time.Now().UTC().Add(-time.Hour),
			Priority:       7,
		}},
		tokenResponse:  oauthTokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 3600},
		profilePayload: profileState{UserID: 1, Username: "agent", ActorID: "https://example.test/users/agent", MainKeyID: "https://example.test/users/agent#main-key"},
	}
	bvServer := newHTTPServer(t, http.HandlerFunc(bv.handler))

	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{handler: func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "final answer"},
				"finish_reason": "stop",
			}},
		}
	}}.serveHTTP))

	statePath := writeAgentStateFile(t, agentState{
		ServerURL:    bvServer.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Model:        "m",
		LMStudioURL:  lm.URL,
		Token: oauthTokenState{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().UTC().Add(30 * time.Minute),
		},
		Profile: profileState{UserID: 1, Username: "agent", ActorID: "https://example.test/users/agent", MainKeyID: "https://example.test/users/agent#main-key"},
	})

	if err := runAgentOnce(runConfig{StatePath: statePath, Workspace: workspace, MaxIter: 3}); err != nil {
		t.Fatalf("runAgentOnce: %v", err)
	}

	bv.mu.Lock()
	defer bv.mu.Unlock()
	if len(bv.comments) != 2 {
		t.Fatalf("expected acknowledgement and completion comments, got %d", len(bv.comments))
	}
	ackComment := bv.comments[0]
	completionComment := bv.comments[1]
	if ackComment.AgentCommentKind != common.AgentCommentKindAck {
		t.Fatalf("unexpected acknowledgement kind: %q", ackComment.AgentCommentKind)
	}
	if !strings.Contains(ackComment.Content, "Agent acknowledged ticket") {
		t.Fatalf("unexpected acknowledgement content: %q", ackComment.Content)
	}
	if completionComment.AgentCommentKind != common.AgentCommentKindCompletion {
		t.Fatalf("unexpected completion kind: %q", completionComment.AgentCommentKind)
	}
	if !strings.Contains(completionComment.Content, "Agent completed ticket") {
		t.Fatalf("unexpected completion content: %q", completionComment.Content)
	}
	if !strings.Contains(completionComment.Content, "\n\nfinal answer") {
		t.Fatalf("expected final answer in completion comment, got %q", completionComment.Content)
	}
	if len(bv.events) < 2 {
		t.Fatalf("expected at least comment + update events, got %v", bv.events)
	}
	if bv.events[0] != "comment" {
		t.Fatalf("expected first event to be comment, got %q", bv.events[0])
	}
	if len(bv.updates) < 2 {
		t.Fatalf("expected start and assistant updates, got %v", bv.updates)
	}
	if got := bv.updates[0].Content; got != "agent: starting ticket processing" {
		t.Fatalf("unexpected first update: %q", got)
	}
	for _, item := range bv.updates {
		if item.CommentSlug != ackComment.Slug {
			t.Fatalf("expected all updates on ack comment %q, got update on %q", ackComment.Slug, item.CommentSlug)
		}
	}
	if !strings.Contains(joinCommentUpdateContents(bv.updates, ackComment.Slug), "assistant:\nfinal answer") {
		t.Fatalf("expected assistant update in updates, got %v", bv.updates)
	}
}

func TestRunAgentOnceTestCounterPublishesDeterministicUpdates(t *testing.T) {
	stubTicketWorkspaceLifecycle(t, nil)

	bv := &fakeBVServer{
		assigned: []common.AssignedTicket{{
			ActorID:        "https://example.test/ticket-tracker/tracker-1/ticket/TCK-COUNTER",
			TrackerSlug:    "tracker-1",
			TicketSlug:     "TCK-COUNTER",
			RepositorySlug: "repo-1",
			Summary:        "Deterministic test",
			Content:        "details",
			CreatedAt:      time.Now().UTC().Add(-time.Hour),
			Priority:       5,
		}},
		tokenResponse:  oauthTokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 3600},
		profilePayload: profileState{UserID: 1, Username: "agent", ActorID: "https://example.test/users/agent", MainKeyID: "https://example.test/users/agent#main-key"},
	}
	bvServer := newHTTPServer(t, http.HandlerFunc(bv.handler))

	statePath := writeAgentStateFile(t, agentState{
		ServerURL:    bvServer.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Mode:         agentModeTestCounter,
		Token: oauthTokenState{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().UTC().Add(30 * time.Minute),
		},
		Profile: profileState{UserID: 1, Username: "agent", ActorID: "https://example.test/users/agent", MainKeyID: "https://example.test/users/agent#main-key"},
	})

	if err := runAgentOnce(runConfig{StatePath: statePath, Workspace: t.TempDir(), MaxIter: 3}); err != nil {
		t.Fatalf("runAgentOnce: %v", err)
	}

	bv.mu.Lock()
	defer bv.mu.Unlock()
	if len(bv.comments) != 2 {
		t.Fatalf("expected acknowledgement and completion comments, got %d", len(bv.comments))
	}
	ackComment := bv.comments[0]
	if ackComment.AgentCommentKind != common.AgentCommentKindAck {
		t.Fatalf("unexpected acknowledgement kind: %q", ackComment.AgentCommentKind)
	}
	if bv.comments[1].AgentCommentKind != common.AgentCommentKindCompletion {
		t.Fatalf("unexpected completion kind: %q", bv.comments[1].AgentCommentKind)
	}
	wantUpdates := []string{"test-step:1", "test-step:2", "test-step:3"}
	if len(bv.updates) != len(wantUpdates) {
		t.Fatalf("expected %d updates, got %d: %v", len(wantUpdates), len(bv.updates), bv.updates)
	}
	for i, want := range wantUpdates {
		if bv.updates[i].CommentSlug != ackComment.Slug {
			t.Fatalf("expected update %d on ack comment %q, got %q", i, ackComment.Slug, bv.updates[i].CommentSlug)
		}
		if bv.updates[i].Content != want {
			t.Fatalf("update %d mismatch: got %q want %q", i, bv.updates[i].Content, want)
		}
	}
}

func TestRunAgentOnceIterationLimitPostsErrorUpdate(t *testing.T) {
	workspace := t.TempDir()
	stubTicketWorkspaceLifecycle(t, map[string]string{"README.txt": "hello\n"})

	bv := &fakeBVServer{
		assigned: []common.AssignedTicket{{
			ActorID:        "https://example.test/ticket-tracker/tracker-1/ticket/TCK-2",
			TrackerSlug:    "tracker-1",
			TicketSlug:     "TCK-2",
			RepositorySlug: "repo-1",
			Summary:        "Fix bug",
			Content:        "details",
			CreatedAt:      time.Now().UTC().Add(-time.Hour),
			Priority:       3,
		}},
		tokenResponse:  oauthTokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 3600},
		profilePayload: profileState{UserID: 1, Username: "agent", ActorID: "https://example.test/users/agent", MainKeyID: "https://example.test/users/agent#main-key"},
	}
	bvServer := newHTTPServer(t, http.HandlerFunc(bv.handler))

	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{handler: func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{{
						"id":   "call-1",
						"type": "function",
						"function": map[string]any{
							"name":      "list_dir",
							"arguments": "{\"path\":\".\"}",
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
		}
	}}.serveHTTP))

	statePath := writeAgentStateFile(t, agentState{
		ServerURL:    bvServer.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Model:        "m",
		LMStudioURL:  lm.URL,
		Token: oauthTokenState{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().UTC().Add(30 * time.Minute),
		},
		Profile: profileState{UserID: 1, Username: "agent", ActorID: "https://example.test/users/agent", MainKeyID: "https://example.test/users/agent#main-key"},
	})

	err := runAgentOnce(runConfig{StatePath: statePath, Workspace: workspace, MaxIter: 1})
	if err == nil {
		t.Fatalf("expected iteration limit error")
	}
	if !strings.Contains(err.Error(), "iteration limit") {
		t.Fatalf("expected iteration limit error, got: %v", err)
	}

	bv.mu.Lock()
	defer bv.mu.Unlock()
	if len(bv.comments) != 1 {
		t.Fatalf("expected one acknowledgement comment, got %d", len(bv.comments))
	}
	ackComment := bv.comments[0]
	if ackComment.AgentCommentKind != common.AgentCommentKindAck {
		t.Fatalf("unexpected acknowledgement kind: %q", ackComment.AgentCommentKind)
	}
	updatesJoined := joinCommentUpdateContents(bv.updates, ackComment.Slug)
	if !strings.Contains(updatesJoined, "tool_call:") {
		t.Fatalf("expected tool_call update, got %v", bv.updates)
	}
	if !strings.Contains(updatesJoined, "tool_result:") {
		t.Fatalf("expected tool_result update, got %v", bv.updates)
	}
	if !strings.Contains(updatesJoined, "agent_error:") {
		t.Fatalf("expected agent_error update, got %v", bv.updates)
	}
	for _, comment := range bv.comments {
		if comment.AgentCommentKind == common.AgentCommentKindCompletion {
			t.Fatalf("did not expect completion comment on iteration limit failure")
		}
	}
}

func TestRunAgentOnceRefreshesTokenAndPersistsState(t *testing.T) {
	bv := &fakeBVServer{
		tokenResponse: oauthTokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			TokenType:    "Bearer",
			Scope:        agentScope,
			ExpiresIn:    1800,
		},
		profilePayload: profileState{
			UserID:    77,
			Username:  "agent",
			ActorID:   "https://example.test/users/agent",
			MainKeyID: "https://example.test/users/agent#main-key",
		},
	}
	bvServer := newHTTPServer(t, http.HandlerFunc(bv.handler))

	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{handler: func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "unused"},
				"finish_reason": "stop",
			}},
		}
	}}.serveHTTP))

	statePath := writeAgentStateFile(t, agentState{
		ServerURL:    bvServer.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Model:        "m",
		LMStudioURL:  lm.URL,
		Token: oauthTokenState{
			AccessToken:  "old-access",
			RefreshToken: "old-refresh",
			ExpiresAt:    time.Now().UTC().Add(-10 * time.Minute),
		},
		Profile: profileState{UserID: 1, Username: "old", ActorID: "https://example.test/users/old", MainKeyID: "https://example.test/users/old#main-key"},
	})

	if err := runAgentOnce(runConfig{StatePath: statePath, Workspace: t.TempDir(), MaxIter: 3}); err != nil {
		t.Fatalf("runAgentOnce: %v", err)
	}

	updatedState, err := loadAgentState(statePath)
	if err != nil {
		t.Fatalf("loadAgentState: %v", err)
	}
	if updatedState.Token.AccessToken != "new-access" {
		t.Fatalf("expected refreshed access token, got %q", updatedState.Token.AccessToken)
	}
	if updatedState.Token.RefreshToken != "new-refresh" {
		t.Fatalf("expected refreshed refresh token, got %q", updatedState.Token.RefreshToken)
	}
	if updatedState.Profile.UserID != 77 {
		t.Fatalf("expected refreshed profile user id 77, got %d", updatedState.Profile.UserID)
	}

	bv.mu.Lock()
	defer bv.mu.Unlock()
	if got := bv.assignedAuthHeader; got != "Bearer new-access" {
		t.Fatalf("expected assigned-ticket request to use refreshed token, got %q", got)
	}
	if !strings.Contains(bv.assignedQuery, "agent_completion_pending=true") {
		t.Fatalf("expected assigned-ticket query to request completion-pending tickets, got %q", bv.assignedQuery)
	}
	if strings.Contains(bv.assignedQuery, "unresponded=") {
		t.Fatalf("did not expect unresponded filter in assigned-ticket query, got %q", bv.assignedQuery)
	}
}

func TestTicketEnvelopeIncludesCreatedAtAndPriority(t *testing.T) {
	ticket := common.AssignedTicket{
		TicketSlug:     "TCK-1234",
		TrackerSlug:    "tracker",
		RepositorySlug: "repo",
		Summary:        "Fix nil pointer",
		Content:        "Malformed JSON to /webhook panics",
		CreatedAt:      time.Date(2026, 2, 28, 12, 30, 0, 0, time.UTC),
		Priority:       42,
	}
	env := ticketEnvelope(ticket)
	for _, want := range []string{
		"id: TCK-1234",
		"tracker: tracker",
		"repository: repo",
		"date: 2026-02-28T12:30:00Z",
		"priority: 42",
		"title: Fix nil pointer",
		"Description:\nMalformed JSON to /webhook panics",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("ticket envelope missing %q:\n%s", want, env)
		}
	}
}

func TestRunAgentOnceFetchesRepoDetailAndUsesTicketCheckoutPath(t *testing.T) {
	workspaceRoot := t.TempDir()
	bv := &fakeBVServer{
		assigned: []common.AssignedTicket{{
			TrackerSlug:    "tracker-1",
			TicketSlug:     "TCK-1",
			RepositorySlug: "repo-1",
			Summary:        "Fix bug",
			Content:        "details",
			CreatedAt:      time.Now().UTC().Add(-time.Hour),
		}},
		repos: map[string]common.Repository{
			"repo-1": {Slug: "repo-1", Path: "/translated/root/repo/repo-1"},
		},
	}
	bvServer := newHTTPServer(t, http.HandlerFunc(bv.handler))

	statePath := writeAgentStateFile(t, agentState{
		ServerURL:    bvServer.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Mode:         agentModeTestCounter,
		Token: oauthTokenState{
			AccessToken: "access",
			ExpiresAt:   time.Now().UTC().Add(30 * time.Minute),
		},
		Profile: profileState{UserID: 1, Username: "agent"},
	})

	origPrepare := prepareTicketWorkspaceForRun
	origFinalize := finalizeTicketWorkspaceForRun
	defer func() {
		prepareTicketWorkspaceForRun = origPrepare
		finalizeTicketWorkspaceForRun = origFinalize
	}()

	var prepared ticketWorkspace
	prepareTicketWorkspaceForRun = func(parent string, ticket common.AssignedTicket, repo common.Repository) (ticketWorkspace, error) {
		prepared = ticketWorkspace{
			Path:              filepath.Join(parent, ticket.RepositorySlug, ticket.TicketSlug),
			SourceRepoPath:    repo.Path,
			BaselineUntracked: map[string]struct{}{},
		}
		return prepared, nil
	}
	finalizeTicketWorkspaceForRun = func(_ string, _ ticketWorkspace, _ common.AssignedTicket) error {
		return nil
	}

	if err := runAgentOnce(runConfig{StatePath: statePath, Workspace: workspaceRoot, MaxIter: 3}); err != nil {
		t.Fatalf("runAgentOnce: %v", err)
	}

	bv.mu.Lock()
	defer bv.mu.Unlock()
	if got := bv.repoDetailAuth; got != "Bearer access" {
		t.Fatalf("repo detail auth header mismatch: got %q", got)
	}
	if len(bv.repoDetailSlugs) != 1 || bv.repoDetailSlugs[0] != "repo-1" {
		t.Fatalf("unexpected repo detail slugs: %v", bv.repoDetailSlugs)
	}
	wantPath := filepath.Join(workspaceRoot, "repo-1", "TCK-1")
	if prepared.Path != wantPath {
		t.Fatalf("checkout path mismatch: got %q want %q", prepared.Path, wantPath)
	}
	if prepared.SourceRepoPath != "/translated/root/repo/repo-1" {
		t.Fatalf("source repo path mismatch: got %q", prepared.SourceRepoPath)
	}
}

func TestRunAgentOnceCheckoutFailurePostsErrorUpdateAndSkipsCompletion(t *testing.T) {
	bv := &fakeBVServer{
		assigned: []common.AssignedTicket{{
			TrackerSlug:    "tracker-1",
			TicketSlug:     "TCK-ERR",
			RepositorySlug: "repo-1",
			Summary:        "Fix bug",
			Content:        "details",
			CreatedAt:      time.Now().UTC().Add(-time.Hour),
		}},
		repos: map[string]common.Repository{
			"repo-1": {Slug: "repo-1", Path: "/translated/root/repo/repo-1"},
		},
	}
	bvServer := newHTTPServer(t, http.HandlerFunc(bv.handler))

	statePath := writeAgentStateFile(t, agentState{
		ServerURL:    bvServer.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Mode:         agentModeTestCounter,
		Token: oauthTokenState{
			AccessToken: "access",
			ExpiresAt:   time.Now().UTC().Add(30 * time.Minute),
		},
		Profile: profileState{UserID: 1, Username: "agent"},
	})

	origPrepare := prepareTicketWorkspaceForRun
	defer func() { prepareTicketWorkspaceForRun = origPrepare }()
	prepareTicketWorkspaceForRun = func(string, common.AssignedTicket, common.Repository) (ticketWorkspace, error) {
		return ticketWorkspace{}, fmt.Errorf("clone failed")
	}

	err := runAgentOnce(runConfig{StatePath: statePath, Workspace: t.TempDir(), MaxIter: 3})
	if err == nil || !strings.Contains(err.Error(), "clone failed") {
		t.Fatalf("expected clone failure, got %v", err)
	}

	bv.mu.Lock()
	defer bv.mu.Unlock()
	if len(bv.comments) != 1 {
		t.Fatalf("expected acknowledgement only, got %d comments", len(bv.comments))
	}
	if len(bv.updates) == 0 || !strings.Contains(joinCommentUpdateContents(bv.updates, bv.comments[0].Slug), "agent_error: clone failed") {
		t.Fatalf("expected agent_error update, got %v", bv.updates)
	}
}

func TestRunAgentOnceCommitFailurePostsErrorUpdateAndSkipsCompletion(t *testing.T) {
	bv := &fakeBVServer{
		assigned: []common.AssignedTicket{{
			TrackerSlug:    "tracker-1",
			TicketSlug:     "TCK-RECORD",
			RepositorySlug: "repo-1",
			Summary:        "Fix bug",
			Content:        "details",
			CreatedAt:      time.Now().UTC().Add(-time.Hour),
		}},
		repos: map[string]common.Repository{
			"repo-1": {Slug: "repo-1", Path: "/translated/root/repo/repo-1"},
		},
	}
	bvServer := newHTTPServer(t, http.HandlerFunc(bv.handler))

	statePath := writeAgentStateFile(t, agentState{
		ServerURL:    bvServer.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "http://127.0.0.1:8787/callback",
		Mode:         agentModeTestCounter,
		Token: oauthTokenState{
			AccessToken: "access",
			ExpiresAt:   time.Now().UTC().Add(30 * time.Minute),
		},
		Profile: profileState{UserID: 1, Username: "agent"},
	})

	origPrepare := prepareTicketWorkspaceForRun
	origFinalize := finalizeTicketWorkspaceForRun
	defer func() {
		prepareTicketWorkspaceForRun = origPrepare
		finalizeTicketWorkspaceForRun = origFinalize
	}()
	prepareTicketWorkspaceForRun = func(parent string, ticket common.AssignedTicket, repo common.Repository) (ticketWorkspace, error) {
		return ticketWorkspace{
			Path:           filepath.Join(parent, ticket.RepositorySlug, ticket.TicketSlug),
			SourceRepoPath: repo.Path,
		}, nil
	}
	finalizeTicketWorkspaceForRun = func(string, ticketWorkspace, common.AssignedTicket) error {
		return fmt.Errorf("record failed")
	}

	err := runAgentOnce(runConfig{StatePath: statePath, Workspace: t.TempDir(), MaxIter: 3})
	if err == nil || !strings.Contains(err.Error(), "record failed") {
		t.Fatalf("expected record failure, got %v", err)
	}

	bv.mu.Lock()
	defer bv.mu.Unlock()
	if len(bv.comments) != 1 {
		t.Fatalf("expected acknowledgement only, got %d comments", len(bv.comments))
	}
	if len(bv.updates) == 0 || !strings.Contains(joinCommentUpdateContents(bv.updates, bv.comments[0].Slug), "agent_error: record failed") {
		t.Fatalf("expected agent_error update, got %v", bv.updates)
	}
}
