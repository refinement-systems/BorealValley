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

package common

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func integrationPostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("BV_TEST_PG_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv(PostgresDSNEnv))
	}
	if dsn == "" {
		t.Skip("set BV_TEST_PG_DSN (or BV_PG_DSN) to run integration tests")
	}
	return dsn
}

func setupControlIntegration(t *testing.T) (*Store, Repository, int64) {
	t.Helper()

	root := filepath.Join(t.TempDir(), "root")
	if err := InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}

	repoName := fmt.Sprintf("repo-%d", time.Now().UnixNano())
	repoDir := filepath.Join(RootRepoPath(root), repoName)
	if err := os.MkdirAll(repoDir, 0o700); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	if out, err := exec.Command("git", "-C", repoDir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	store, err := StoreInit(integrationPostgresDSN(t), root)
	if err != nil {
		t.Fatalf("StoreInit: %v", err)
	}
	t.Cleanup(store.Close)

	if err := store.ResyncFromFilesystem(context.Background()); err != nil {
		t.Fatalf("ResyncFromFilesystem: %v", err)
	}

	userID, _ := createAndVerifyIntegrationUser(t, store, "integration-user", false)

	repoSlug := slugify(repoName)
	repo, found, err := store.GetRepositoryBySlug(context.Background(), repoSlug)
	if err != nil {
		t.Fatalf("GetRepositoryBySlug: %v", err)
	}
	if !found {
		t.Fatalf("repository not found: %s", repoSlug)
	}

	return store, repo, userID
}

func createAndVerifyIntegrationUser(t *testing.T, store *Store, prefix string, isAdmin bool) (int64, string) {
	t.Helper()

	username := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	const password = "integration-password"
	if err := store.CreateUserWithAdmin(context.Background(), username, password, isAdmin); err != nil {
		t.Fatalf("CreateUserWithAdmin: %v", err)
	}
	userID, ok, err := store.VerifyUser(context.Background(), username, password)
	if err != nil {
		t.Fatalf("VerifyUser: %v", err)
	}
	if !ok {
		t.Fatal("expected VerifyUser success")
	}
	return userID, username
}

func loadObjectBodyBySlug(t *testing.T, store *Store, table, slug string) map[string]any {
	t.Helper()

	query := fmt.Sprintf("SELECT body FROM %s WHERE slug = $1", table)
	var raw []byte
	if err := store.db.QueryRowContext(context.Background(), query, slug).Scan(&raw); err != nil {
		t.Fatalf("load %s body: %v", table, err)
	}
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal %s body: %v", table, err)
	}
	return body
}

func TestIntegrationAssignUnassignTicketTrackerComposedFields(t *testing.T) {
	store, repo, userID := setupControlIntegration(t)
	ctx := context.Background()

	trackerName := fmt.Sprintf("tracker-%d", time.Now().UnixNano())
	tracker, err := store.CreateTicketTracker(ctx, userID, trackerName, "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}

	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}

	repoBody := loadObjectBodyBySlug(t, store, "ff_repository", repo.Slug)
	if _, ok := repoBody["ticketsTrackedBy"]; ok {
		t.Fatalf("repository ticketsTrackedBy should not be persisted")
	}

	trackerBody := loadObjectBodyBySlug(t, store, "ff_ticket_tracker", tracker.Slug)
	if _, ok := trackerBody["tracksTicketsFor"]; ok {
		t.Fatalf("tracker tracksTicketsFor should not be persisted")
	}

	repoObj, found, err := store.GetLocalRepositoryObjectBySlug(ctx, repo.Slug)
	if err != nil {
		t.Fatalf("GetLocalRepositoryObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected local repository object")
	}
	repoObjBody, err := parseBody(repoObj.BodyJSON)
	if err != nil {
		t.Fatalf("parse repository object body: %v", err)
	}
	if got := strings.TrimSpace(stringFromAny(repoObjBody["ticketsTrackedBy"])); got != tracker.ActorID {
		t.Fatalf("composed repository ticketsTrackedBy mismatch: got %q want %q", got, tracker.ActorID)
	}

	trackerObj, found, err := store.GetLocalTicketTrackerObjectBySlug(ctx, tracker.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketTrackerObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected local tracker object")
	}
	trackerObjBody, err := parseBody(trackerObj.BodyJSON)
	if err != nil {
		t.Fatalf("parse tracker object body: %v", err)
	}
	if !containsString(stringSliceFromAny(trackerObjBody["tracksTicketsFor"]), repo.ActorID) {
		t.Fatalf("composed tracker tracksTicketsFor missing repo actor %q", repo.ActorID)
	}

	if err := store.UnassignTicketTrackerFromRepository(ctx, repo.Slug); err != nil {
		t.Fatalf("UnassignTicketTrackerFromRepository: %v", err)
	}

	repoBody = loadObjectBodyBySlug(t, store, "ff_repository", repo.Slug)
	if _, ok := repoBody["ticketsTrackedBy"]; ok {
		t.Fatalf("repository ticketsTrackedBy should not be persisted")
	}

	trackerBody = loadObjectBodyBySlug(t, store, "ff_ticket_tracker", tracker.Slug)
	if _, ok := trackerBody["tracksTicketsFor"]; ok {
		t.Fatalf("tracker tracksTicketsFor should not be persisted")
	}

	repoObj, found, err = store.GetLocalRepositoryObjectBySlug(ctx, repo.Slug)
	if err != nil {
		t.Fatalf("GetLocalRepositoryObjectBySlug after unassign: %v", err)
	}
	if !found {
		t.Fatalf("expected local repository object after unassign")
	}
	repoObjBody, err = parseBody(repoObj.BodyJSON)
	if err != nil {
		t.Fatalf("parse repository object body after unassign: %v", err)
	}
	if _, ok := repoObjBody["ticketsTrackedBy"]; ok {
		t.Fatalf("composed ticketsTrackedBy should be absent after unassign")
	}

	trackerObj, found, err = store.GetLocalTicketTrackerObjectBySlug(ctx, tracker.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketTrackerObjectBySlug after unassign: %v", err)
	}
	if !found {
		t.Fatalf("expected local tracker object after unassign")
	}
	trackerObjBody, err = parseBody(trackerObj.BodyJSON)
	if err != nil {
		t.Fatalf("parse tracker object body after unassign: %v", err)
	}
	if containsString(stringSliceFromAny(trackerObjBody["tracksTicketsFor"]), repo.ActorID) {
		t.Fatalf("composed tracksTicketsFor still contains repo actor after unassign")
	}
}

func TestIntegrationCreateTicketUsesRelationalTrackerLinkOnly(t *testing.T) {
	store, repo, userID := setupControlIntegration(t)
	ctx := context.Background()

	trackerName := fmt.Sprintf("tracker-%d", time.Now().UnixNano())
	tracker, err := store.CreateTicketTracker(ctx, userID, trackerName, "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}

	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}

	brokenRepoBody := loadObjectBodyBySlug(t, store, "ff_repository", repo.Slug)
	brokenRepoBody["ticketsTrackedBy"] = "https://example.invalid/not-the-tracker"
	brokenRaw, err := json.Marshal(brokenRepoBody)
	if err != nil {
		t.Fatalf("marshal repository body: %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(),
		`UPDATE ff_repository SET body = $1::jsonb, updated_at = now() WHERE slug = $2`,
		string(brokenRaw), repo.Slug,
	); err != nil {
		t.Fatalf("tamper repository body: %v", err)
	}

	ticket, err := store.CreateTicket(ctx, userID, tracker.Slug, repo.Slug, "Summary", "Content")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if ticket.Slug == "" {
		t.Fatalf("expected non-empty ticket slug")
	}

	if _, statErr := os.Stat(filepath.Join(repo.Path, ".task")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no .task directory from ticket creation, stat err: %v", statErr)
	}
}

func TestIntegrationCanonicalObjectLookupMethods(t *testing.T) {
	store, repo, userID := setupControlIntegration(t)

	tracker, err := store.CreateTicketTracker(context.Background(), userID, "tracker canonical", "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := store.AssignTicketTrackerToRepository(context.Background(), repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}
	ticket, err := store.CreateTicket(context.Background(), userID, tracker.Slug, repo.Slug, "Summary", "Content")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	repoObj, found, err := store.GetLocalRepositoryObjectBySlug(context.Background(), repo.Slug)
	if err != nil {
		t.Fatalf("GetLocalRepositoryObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected repository object for %q", repo.Slug)
	}
	if repoObj.PrimaryKey != repo.ActorID {
		t.Fatalf("repository primary key mismatch: got %q want %q", repoObj.PrimaryKey, repo.ActorID)
	}

	trackerObj, found, err := store.GetLocalTicketTrackerObjectBySlug(context.Background(), tracker.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketTrackerObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected tracker object for %q", tracker.Slug)
	}
	if trackerObj.PrimaryKey != tracker.ActorID {
		t.Fatalf("tracker primary key mismatch: got %q want %q", trackerObj.PrimaryKey, tracker.ActorID)
	}

	ticketObj, found, err := store.GetLocalTicketObjectBySlug(context.Background(), tracker.Slug, ticket.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected ticket object for %q", ticket.Slug)
	}
	if ticketObj.PrimaryKey != ticket.ActorID {
		t.Fatalf("ticket primary key mismatch: got %q want %q", ticketObj.PrimaryKey, ticket.ActorID)
	}
	if ticketObj.RepositorySlug != repo.Slug {
		t.Fatalf("ticket repository slug mismatch: got %q want %q", ticketObj.RepositorySlug, repo.Slug)
	}

	if _, found, err := store.GetLocalTicketObjectBySlug(context.Background(), tracker.Slug, "missing-ticket"); err != nil {
		t.Fatalf("GetLocalTicketObjectBySlug missing ticket: %v", err)
	} else if found {
		t.Fatal("expected missing ticket lookup to return found=false")
	}

	if _, found, err := store.GetLocalTicketObjectBySlug(context.Background(), "wrong-tracker", ticket.Slug); err != nil {
		t.Fatalf("GetLocalTicketObjectBySlug wrong tracker: %v", err)
	} else if found {
		t.Fatal("expected wrong tracker lookup to return found=false")
	}
}

func TestIntegrationCreateTicketCommentStoresAsNoteAndEnforcesScope(t *testing.T) {
	store, repo, userID := setupControlIntegration(t)
	ctx := context.Background()

	tracker, err := store.CreateTicketTracker(ctx, userID, "comment tracker", "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}
	ticket, err := store.CreateTicket(ctx, userID, tracker.Slug, repo.Slug, "summary", "content")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	comment, err := store.CreateTicketComment(ctx, userID, tracker.Slug, ticket.Slug, "first comment", "")
	if err != nil {
		t.Fatalf("CreateTicketComment: %v", err)
	}
	if comment.ActorID == "" {
		t.Fatalf("expected comment actor id")
	}

	var (
		noteType string
		rawBody  []byte
	)
	if err := store.db.QueryRowContext(ctx,
		`SELECT type, body FROM as_note WHERE primary_key = $1`,
		comment.ActorID,
	).Scan(&noteType, &rawBody); err != nil {
		t.Fatalf("query as_note: %v", err)
	}
	if noteType != "Note" {
		t.Fatalf("expected as_note.type=Note, got %q", noteType)
	}
	body := map[string]any{}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("unmarshal note body: %v", err)
	}
	if got := stringFromAny(body["content"]); got != "first comment" {
		t.Fatalf("expected comment content in as_note, got %q", got)
	}

	var (
		recipientActorID string
		inReplyToPK      sql.NullString
	)
	if err := store.db.QueryRowContext(ctx,
		`SELECT recipient_actor_id, in_reply_to_note_primary_key
		   FROM ff_ticket_comment
		  WHERE note_primary_key = $1`,
		comment.ActorID,
	).Scan(&recipientActorID, &inReplyToPK); err != nil {
		t.Fatalf("query ff_ticket_comment: %v", err)
	}
	if recipientActorID != repo.ActorID {
		t.Fatalf("expected recipient_actor_id %q, got %q", repo.ActorID, recipientActorID)
	}
	if inReplyToPK.Valid {
		t.Fatalf("expected root comment to have NULL in_reply_to_note_primary_key, got %q", inReplyToPK.String)
	}

	reply, err := store.CreateTicketComment(ctx, userID, tracker.Slug, ticket.Slug, "reply", comment.ActorID)
	if err != nil {
		t.Fatalf("CreateTicketComment reply: %v", err)
	}
	if reply.InReplyToActorID != comment.ActorID {
		t.Fatalf("expected reply parent %q, got %q", comment.ActorID, reply.InReplyToActorID)
	}

	otherTicket, err := store.CreateTicket(ctx, userID, tracker.Slug, repo.Slug, "other", "other content")
	if err != nil {
		t.Fatalf("CreateTicket other: %v", err)
	}
	foreignComment, err := store.CreateTicketComment(ctx, userID, tracker.Slug, otherTicket.Slug, "other comment", "")
	if err != nil {
		t.Fatalf("CreateTicketComment other: %v", err)
	}
	if _, err := store.CreateTicketComment(ctx, userID, tracker.Slug, ticket.Slug, "cross reply", foreignComment.ActorID); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation for cross-ticket reply, got %v", err)
	}

	if _, err := store.db.ExecContext(ctx,
		`UPDATE ff_ticket_comment
		    SET recipient_actor_id = $1
		  WHERE note_primary_key = $2`,
		"https://example.invalid/different-recipient",
		comment.ActorID,
	); err != nil {
		t.Fatalf("tamper recipient_actor_id: %v", err)
	}
	if _, err := store.CreateTicketComment(ctx, userID, tracker.Slug, ticket.Slug, "recipient mismatch", comment.ActorID); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation for recipient mismatch reply, got %v", err)
	}
}

func TestIntegrationCreateTicketCommentStoresAgentCommentKind(t *testing.T) {
	store, repo, userID := setupControlIntegration(t)
	ctx := context.Background()

	tracker, err := store.CreateTicketTracker(ctx, userID, "agent kind tracker", "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}
	ticket, err := store.CreateTicket(ctx, userID, tracker.Slug, repo.Slug, "summary", "content")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	ackComment, err := store.CreateAgentTicketComment(ctx, userID, tracker.Slug, ticket.Slug, "Agent acknowledged ticket.", "", AgentCommentKindAck)
	if err != nil {
		t.Fatalf("CreateAgentTicketComment ack: %v", err)
	}
	completionComment, err := store.CreateAgentTicketComment(ctx, userID, tracker.Slug, ticket.Slug, "Agent completed ticket.", "", AgentCommentKindCompletion)
	if err != nil {
		t.Fatalf("CreateAgentTicketComment completion: %v", err)
	}
	plainComment, err := store.CreateTicketComment(ctx, userID, tracker.Slug, ticket.Slug, "plain comment", "")
	if err != nil {
		t.Fatalf("CreateTicketComment plain: %v", err)
	}

	tests := []struct {
		name       string
		actorID    string
		wantKind   string
		wantHasKey bool
	}{
		{name: "ack", actorID: ackComment.ActorID, wantKind: AgentCommentKindAck, wantHasKey: true},
		{name: "completion", actorID: completionComment.ActorID, wantKind: AgentCommentKindCompletion, wantHasKey: true},
		{name: "plain", actorID: plainComment.ActorID, wantKind: "", wantHasKey: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var rawBody []byte
			if err := store.db.QueryRowContext(ctx,
				`SELECT body FROM as_note WHERE primary_key = $1`,
				tc.actorID,
			).Scan(&rawBody); err != nil {
				t.Fatalf("query as_note: %v", err)
			}
			body := map[string]any{}
			if err := json.Unmarshal(rawBody, &body); err != nil {
				t.Fatalf("unmarshal note body: %v", err)
			}
			got, hasKey := body[AgentCommentKindField]
			if hasKey != tc.wantHasKey {
				t.Fatalf("kind key presence mismatch: got %v want %v", hasKey, tc.wantHasKey)
			}
			if gotKind := stringFromAny(got); gotKind != tc.wantKind {
				t.Fatalf("agent comment kind mismatch: got %q want %q", gotKind, tc.wantKind)
			}
		})
	}
}

func TestIntegrationRepositoryMembershipACLAndAdminBypass(t *testing.T) {
	store, repo, ownerID := setupControlIntegration(t)
	ctx := context.Background()

	tracker, err := store.CreateTicketTracker(ctx, ownerID, "member tracker", "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}
	ticket, err := store.CreateTicket(ctx, ownerID, tracker.Slug, repo.Slug, "membership", "membership content")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	isMember, err := store.IsRepositoryMember(ctx, repo.Slug, ownerID)
	if err != nil {
		t.Fatalf("IsRepositoryMember owner: %v", err)
	}
	if !isMember {
		t.Fatalf("expected ticket creator to be auto-added as repository member")
	}

	outsiderID, outsiderName := createAndVerifyIntegrationUser(t, store, "outsider", false)
	canAccess, err := store.CanAccessRepository(ctx, repo.Slug, outsiderID)
	if err != nil {
		t.Fatalf("CanAccessRepository outsider: %v", err)
	}
	if canAccess {
		t.Fatalf("expected outsider to have no repository access")
	}
	if _, err := store.CreateTicketComment(ctx, outsiderID, tracker.Slug, ticket.Slug, "denied", ""); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation for outsider comment write, got %v", err)
	}

	adminID, _ := createAndVerifyIntegrationUser(t, store, "admin", true)
	canAccess, err = store.CanAccessRepository(ctx, repo.Slug, adminID)
	if err != nil {
		t.Fatalf("CanAccessRepository admin: %v", err)
	}
	if !canAccess {
		t.Fatalf("expected admin bypass repository ACL")
	}
	if _, err := store.CreateTicketComment(ctx, adminID, tracker.Slug, ticket.Slug, "admin note", ""); err != nil {
		t.Fatalf("expected admin to write comment: %v", err)
	}

	if err := store.AddRepositoryMemberByUsername(ctx, repo.Slug, outsiderName); err != nil {
		t.Fatalf("AddRepositoryMemberByUsername: %v", err)
	}
	isMember, err = store.IsRepositoryMember(ctx, repo.Slug, outsiderID)
	if err != nil {
		t.Fatalf("IsRepositoryMember outsider after add: %v", err)
	}
	if !isMember {
		t.Fatalf("expected outsider to be member after add")
	}

	if err := store.RemoveRepositoryMemberByUsername(ctx, repo.Slug, outsiderName); err != nil {
		t.Fatalf("RemoveRepositoryMemberByUsername: %v", err)
	}
	isMember, err = store.IsRepositoryMember(ctx, repo.Slug, outsiderID)
	if err != nil {
		t.Fatalf("IsRepositoryMember outsider after remove: %v", err)
	}
	if isMember {
		t.Fatalf("expected outsider membership removal")
	}
}

func TestIntegrationTicketAssignmentNotifications(t *testing.T) {
	store, repo, ownerID := setupControlIntegration(t)
	ctx := context.Background()

	tracker, err := store.CreateTicketTracker(ctx, ownerID, "notify tracker", "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}
	ticket, err := store.CreateTicket(ctx, ownerID, tracker.Slug, repo.Slug, "notify summary", "notify content")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	assigneeID, assigneeName := createAndVerifyIntegrationUser(t, store, "assignee", false)
	assigneeRecord, found, err := store.GetUserActorByUsername(ctx, assigneeName)
	if err != nil {
		t.Fatalf("GetUserActorByUsername assignee: %v", err)
	}
	if !found {
		t.Fatalf("expected assignee actor")
	}
	outsiderID, outsiderName := createAndVerifyIntegrationUser(t, store, "actor-outsider", false)

	if err := store.UpdateTicketAssigneeByUsername(ctx, ownerID, tracker.Slug, ticket.Slug, "add", assigneeName); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error for non-member assignee, got %v", err)
	}
	if err := store.AddRepositoryMemberByUsername(ctx, repo.Slug, assigneeName); err != nil {
		t.Fatalf("AddRepositoryMemberByUsername assignee: %v", err)
	}

	if err := store.UpdateTicketAssigneeByUsername(ctx, outsiderID, tracker.Slug, ticket.Slug, "add", assigneeName); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error for actor without repository access, got %v", err)
	}

	if err := store.UpdateTicketAssigneeByUsername(ctx, ownerID, tracker.Slug, ticket.Slug, "add", assigneeName); err != nil {
		t.Fatalf("UpdateTicketAssigneeByUsername add: %v", err)
	}
	assignees, err := store.ListTicketAssigneesForTicket(ctx, tracker.Slug, ticket.Slug)
	if err != nil {
		t.Fatalf("ListTicketAssigneesForTicket: %v", err)
	}
	if len(assignees) != 1 {
		t.Fatalf("expected 1 assignee, got %d", len(assignees))
	}
	if assignees[0].UserID != assigneeID {
		t.Fatalf("assignee id mismatch: got %d want %d", assignees[0].UserID, assigneeID)
	}

	ticketBody := loadObjectBodyBySlug(t, store, "ff_ticket", ticket.Slug)
	if _, ok := ticketBody["assignedTo"]; ok {
		t.Fatalf("ticket assignedTo should not be persisted in ff_ticket.body")
	}
	ticketObj, found, err := store.GetLocalTicketObjectBySlug(ctx, tracker.Slug, ticket.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected local ticket object")
	}
	ticketObjBody, err := parseBody(ticketObj.BodyJSON)
	if err != nil {
		t.Fatalf("parse local ticket object body: %v", err)
	}
	assignedTo := stringSliceFromAny(ticketObjBody["assignedTo"])
	if !containsString(assignedTo, assigneeRecord.ActorID) {
		t.Fatalf("expected composed ticket assignedTo to contain %q", assigneeRecord.ActorID)
	}

	notifications, hasMore, err := store.ListNotificationsForUser(ctx, assigneeID, NotificationListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListNotificationsForUser initial: %v", err)
	}
	if hasMore {
		t.Fatalf("expected hasMore=false with one notification")
	}
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Type != "ticket_assigned" {
		t.Fatalf("notification type mismatch: got %q", notifications[0].Type)
	}
	if !notifications[0].Unread {
		t.Fatalf("expected notification unread=true initially")
	}
	if notifications[0].Account.Username == "" {
		t.Fatalf("expected account username in notification")
	}

	if err := store.UpdateTicketAssigneeByUsername(ctx, ownerID, tracker.Slug, ticket.Slug, "add", assigneeName); err != nil {
		t.Fatalf("UpdateTicketAssigneeByUsername idempotent add: %v", err)
	}
	notifications, _, err = store.ListNotificationsForUser(ctx, assigneeID, NotificationListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListNotificationsForUser after idempotent add: %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("expected no duplicate notification, got %d", len(notifications))
	}

	notificationID := notifications[0].ID
	if err := store.SetNotificationUnread(ctx, assigneeID, notificationID, false); err != nil {
		t.Fatalf("SetNotificationUnread false: %v", err)
	}
	notifications, _, err = store.ListNotificationsForUser(ctx, assigneeID, NotificationListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListNotificationsForUser after set false: %v", err)
	}
	if notifications[0].Unread {
		t.Fatalf("expected notification unread=false after set")
	}

	if err := store.SetNotificationUnread(ctx, assigneeID, notificationID, true); err != nil {
		t.Fatalf("SetNotificationUnread true: %v", err)
	}
	if err := store.SetAllNotificationsUnread(ctx, assigneeID, false); err != nil {
		t.Fatalf("SetAllNotificationsUnread false: %v", err)
	}
	notifications, _, err = store.ListNotificationsForUser(ctx, assigneeID, NotificationListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListNotificationsForUser after clear: %v", err)
	}
	if notifications[0].Unread {
		t.Fatalf("expected notification unread=false after clear")
	}
	if err := store.SetAllNotificationsUnread(ctx, assigneeID, true); err != nil {
		t.Fatalf("SetAllNotificationsUnread true: %v", err)
	}
	notifications, _, err = store.ListNotificationsForUser(ctx, assigneeID, NotificationListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListNotificationsForUser after reset: %v", err)
	}
	if !notifications[0].Unread {
		t.Fatalf("expected notification unread=true after reset")
	}

	if err := store.UpdateTicketAssigneeByUsername(ctx, ownerID, tracker.Slug, ticket.Slug, "remove", assigneeName); err != nil {
		t.Fatalf("UpdateTicketAssigneeByUsername remove: %v", err)
	}
	if err := store.UpdateTicketAssigneeByUsername(ctx, ownerID, tracker.Slug, ticket.Slug, "add", assigneeName); err != nil {
		t.Fatalf("UpdateTicketAssigneeByUsername add second time: %v", err)
	}
	notifications, _, err = store.ListNotificationsForUser(ctx, assigneeID, NotificationListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListNotificationsForUser after remove+add: %v", err)
	}
	if len(notifications) != 2 {
		t.Fatalf("expected second notification after remove+add, got %d", len(notifications))
	}

	if err := store.AddRepositoryMemberByUsername(ctx, repo.Slug, outsiderName); err != nil {
		t.Fatalf("AddRepositoryMemberByUsername outsider actor: %v", err)
	}
	if err := store.RemoveRepositoryMemberByUsername(ctx, repo.Slug, assigneeName); err != nil {
		t.Fatalf("RemoveRepositoryMemberByUsername assignee: %v", err)
	}
	notifications, _, err = store.ListNotificationsForUser(ctx, assigneeID, NotificationListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListNotificationsForUser after assignee access removal: %v", err)
	}
	if len(notifications) != 0 {
		t.Fatalf("expected notifications to be hidden after repository access removal, got %d", len(notifications))
	}
}

func TestIntegrationTicketPriorityDefaultsAndExplicitPersist(t *testing.T) {
	store, repo, userID := setupControlIntegration(t)
	ctx := context.Background()

	tracker, err := store.CreateTicketTracker(ctx, userID, "priority tracker", "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}

	defaultTicket, err := store.CreateTicket(ctx, userID, tracker.Slug, repo.Slug, "default priority", "default content")
	if err != nil {
		t.Fatalf("CreateTicket default priority: %v", err)
	}
	if defaultTicket.Priority != 0 {
		t.Fatalf("expected default priority 0, got %d", defaultTicket.Priority)
	}

	explicitTicket, err := store.CreateTicketWithPriority(ctx, userID, tracker.Slug, repo.Slug, "explicit priority", "explicit content", 9)
	if err != nil {
		t.Fatalf("CreateTicketWithPriority: %v", err)
	}
	if explicitTicket.Priority != 9 {
		t.Fatalf("expected explicit priority 9, got %d", explicitTicket.Priority)
	}

	var (
		defaultPriority  int
		explicitPriority int
	)
	if err := store.db.QueryRowContext(ctx, `SELECT priority FROM ff_ticket WHERE slug = $1`, defaultTicket.Slug).Scan(&defaultPriority); err != nil {
		t.Fatalf("query default priority: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT priority FROM ff_ticket WHERE slug = $1`, explicitTicket.Slug).Scan(&explicitPriority); err != nil {
		t.Fatalf("query explicit priority: %v", err)
	}
	if defaultPriority != 0 {
		t.Fatalf("expected persisted default priority 0, got %d", defaultPriority)
	}
	if explicitPriority != 9 {
		t.Fatalf("expected persisted explicit priority 9, got %d", explicitPriority)
	}

	explicitBody := loadObjectBodyBySlug(t, store, "ff_ticket", explicitTicket.Slug)
	if _, ok := explicitBody["priority"]; ok {
		t.Fatalf("ticket priority should not be persisted in ff_ticket.body")
	}

	ticketObj, found, err := store.GetLocalTicketObjectBySlug(ctx, tracker.Slug, explicitTicket.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected local ticket object")
	}
	ticketBody, err := parseBody(ticketObj.BodyJSON)
	if err != nil {
		t.Fatalf("parse local ticket object body: %v", err)
	}

	priorityValue, ok := ticketBody["priority"]
	if !ok {
		t.Fatalf("expected composed priority in local ticket object")
	}
	gotPriority, ok := priorityValue.(float64)
	if !ok {
		t.Fatalf("expected composed priority as number, got %T", priorityValue)
	}
	if int(gotPriority) != 9 {
		t.Fatalf("expected composed priority 9, got %.0f", gotPriority)
	}

	tamperedBody := loadObjectBodyBySlug(t, store, "ff_ticket", explicitTicket.Slug)
	tamperedBody["priority"] = 1234
	tamperedRaw, err := json.Marshal(tamperedBody)
	if err != nil {
		t.Fatalf("marshal tampered ticket body: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE ff_ticket
		    SET body = $1::jsonb,
		        updated_at = now()
		  WHERE slug = $2`,
		string(tamperedRaw),
		explicitTicket.Slug,
	); err != nil {
		t.Fatalf("tamper ticket body priority: %v", err)
	}

	ticketObj, found, err = store.GetLocalTicketObjectBySlug(ctx, tracker.Slug, explicitTicket.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketObjectBySlug after tamper: %v", err)
	}
	if !found {
		t.Fatalf("expected local ticket object after tamper")
	}
	ticketBody, err = parseBody(ticketObj.BodyJSON)
	if err != nil {
		t.Fatalf("parse local ticket object body after tamper: %v", err)
	}
	priorityValue, ok = ticketBody["priority"]
	if !ok {
		t.Fatalf("expected composed priority in local ticket object after tamper")
	}
	gotPriority, ok = priorityValue.(float64)
	if !ok {
		t.Fatalf("expected composed priority as number after tamper, got %T", priorityValue)
	}
	if int(gotPriority) != 9 {
		t.Fatalf("expected composed priority 9 after tamper, got %.0f", gotPriority)
	}
}

func TestIntegrationListAssignedTicketsOrderingAndRespondedFlag(t *testing.T) {
	store, repo, ownerID := setupControlIntegration(t)
	ctx := context.Background()

	tracker, err := store.CreateTicketTracker(ctx, ownerID, "assigned order tracker", "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}

	assigneeID, assigneeName := createAndVerifyIntegrationUser(t, store, "assignee-order", false)
	if err := store.AddRepositoryMemberByUsername(ctx, repo.Slug, assigneeName); err != nil {
		t.Fatalf("AddRepositoryMemberByUsername assignee: %v", err)
	}

	ticketOld, err := store.CreateTicketWithPriority(ctx, ownerID, tracker.Slug, repo.Slug, "old", "old content", 0)
	if err != nil {
		t.Fatalf("CreateTicket old: %v", err)
	}
	ticketMidLow, err := store.CreateTicketWithPriority(ctx, ownerID, tracker.Slug, repo.Slug, "mid-low", "mid-low content", 1)
	if err != nil {
		t.Fatalf("CreateTicket mid-low: %v", err)
	}
	ticketMidHigh, err := store.CreateTicketWithPriority(ctx, ownerID, tracker.Slug, repo.Slug, "mid-high", "mid-high content", 9)
	if err != nil {
		t.Fatalf("CreateTicket mid-high: %v", err)
	}

	oldCreated := time.Now().UTC().Add(-2 * time.Hour)
	midCreated := time.Now().UTC().Add(-1 * time.Hour)
	for _, item := range []struct {
		slug      string
		createdAt time.Time
	}{
		{slug: ticketOld.Slug, createdAt: oldCreated},
		{slug: ticketMidLow.Slug, createdAt: midCreated},
		{slug: ticketMidHigh.Slug, createdAt: midCreated},
	} {
		if _, err := store.db.ExecContext(ctx,
			`UPDATE ff_ticket
			    SET created_at = $1,
			        updated_at = now()
			  WHERE slug = $2`,
			item.createdAt,
			item.slug,
		); err != nil {
			t.Fatalf("update ticket timestamp %s: %v", item.slug, err)
		}
	}

	for _, slug := range []string{ticketOld.Slug, ticketMidLow.Slug, ticketMidHigh.Slug} {
		if err := store.UpdateTicketAssigneeByUsername(ctx, ownerID, tracker.Slug, slug, "add", assigneeName); err != nil {
			t.Fatalf("assign ticket %s: %v", slug, err)
		}
	}

	assigned, err := store.ListAssignedTicketsForUser(ctx, assigneeID, AssignedTicketListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListAssignedTicketsForUser: %v", err)
	}
	if len(assigned) != 3 {
		t.Fatalf("expected 3 assigned tickets, got %d", len(assigned))
	}
	orderedSlugs := []string{assigned[0].TicketSlug, assigned[1].TicketSlug, assigned[2].TicketSlug}
	wantSlugs := []string{ticketOld.Slug, ticketMidHigh.Slug, ticketMidLow.Slug}
	for idx := range wantSlugs {
		if orderedSlugs[idx] != wantSlugs[idx] {
			t.Fatalf("ticket order mismatch at index %d: got %q want %q (full=%v)", idx, orderedSlugs[idx], wantSlugs[idx], orderedSlugs)
		}
	}
	for _, item := range assigned {
		if item.RespondedByMe {
			t.Fatalf("expected responded_by_me=false before assignee comment, ticket=%s", item.TicketSlug)
		}
	}

	if _, err := store.CreateTicketComment(ctx, assigneeID, tracker.Slug, ticketMidHigh.Slug, "I am on it", ""); err != nil {
		t.Fatalf("CreateTicketComment assignee: %v", err)
	}

	assigned, err = store.ListAssignedTicketsForUser(ctx, assigneeID, AssignedTicketListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListAssignedTicketsForUser after comment: %v", err)
	}
	responded := map[string]bool{}
	for _, item := range assigned {
		responded[item.TicketSlug] = item.RespondedByMe
	}
	if !responded[ticketMidHigh.Slug] {
		t.Fatalf("expected responded_by_me=true for %s", ticketMidHigh.Slug)
	}
	if responded[ticketOld.Slug] {
		t.Fatalf("expected responded_by_me=false for %s", ticketOld.Slug)
	}
	if responded[ticketMidLow.Slug] {
		t.Fatalf("expected responded_by_me=false for %s", ticketMidLow.Slug)
	}

	unrespondedOnly, err := store.ListAssignedTicketsForUser(ctx, assigneeID, AssignedTicketListOptions{Limit: 10, UnrespondedOnly: true})
	if err != nil {
		t.Fatalf("ListAssignedTicketsForUser unresponded: %v", err)
	}
	if len(unrespondedOnly) != 2 {
		t.Fatalf("expected 2 unresponded tickets, got %d", len(unrespondedOnly))
	}
	for _, item := range unrespondedOnly {
		if item.TicketSlug == ticketMidHigh.Slug {
			t.Fatalf("expected responded ticket %s to be excluded from unresponded list", ticketMidHigh.Slug)
		}
	}

	completionPending, err := store.ListAssignedTicketsForUser(ctx, assigneeID, AssignedTicketListOptions{
		Limit:                      10,
		AgentCompletionPendingOnly: true,
	})
	if err != nil {
		t.Fatalf("ListAssignedTicketsForUser completion pending after plain comment: %v", err)
	}
	if len(completionPending) != 3 {
		t.Fatalf("expected 3 completion-pending tickets after plain comment, got %d", len(completionPending))
	}

	if _, err := store.CreateAgentTicketComment(ctx, assigneeID, tracker.Slug, ticketMidHigh.Slug, "Agent completed ticket.", "", AgentCommentKindCompletion); err != nil {
		t.Fatalf("CreateAgentTicketComment completion: %v", err)
	}

	completionPending, err = store.ListAssignedTicketsForUser(ctx, assigneeID, AssignedTicketListOptions{
		Limit:                      10,
		AgentCompletionPendingOnly: true,
	})
	if err != nil {
		t.Fatalf("ListAssignedTicketsForUser completion pending after completion: %v", err)
	}
	if len(completionPending) != 2 {
		t.Fatalf("expected 2 completion-pending tickets after completion comment, got %d", len(completionPending))
	}
	for _, item := range completionPending {
		if item.TicketSlug == ticketMidHigh.Slug {
			t.Fatalf("expected completed ticket %s to be excluded from completion-pending list", ticketMidHigh.Slug)
		}
	}
}

func TestIntegrationTicketAndCommentUpdateVersionHistory(t *testing.T) {
	store, repo, userID := setupControlIntegration(t)
	ctx := context.Background()

	tracker, err := store.CreateTicketTracker(ctx, userID, "update tracker", "")
	if err != nil {
		t.Fatalf("CreateTicketTracker: %v", err)
	}
	if err := store.AssignTicketTrackerToRepository(ctx, repo.Slug, tracker.Slug); err != nil {
		t.Fatalf("AssignTicketTrackerToRepository: %v", err)
	}
	ticket, err := store.CreateTicket(ctx, userID, tracker.Slug, repo.Slug, "update summary", "base ticket content")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	comment, err := store.CreateTicketComment(ctx, userID, tracker.Slug, ticket.Slug, "base comment content", "")
	if err != nil {
		t.Fatalf("CreateTicketComment: %v", err)
	}

	if _, err := store.CreateTicketUpdate(ctx, userID, tracker.Slug, ticket.Slug, "ticket update one"); err != nil {
		t.Fatalf("CreateTicketUpdate one: %v", err)
	}
	if _, err := store.CreateTicketUpdate(ctx, userID, tracker.Slug, ticket.Slug, "ticket update two"); err != nil {
		t.Fatalf("CreateTicketUpdate two: %v", err)
	}

	ticketObj, found, err := store.GetLocalTicketObjectBySlug(ctx, tracker.Slug, ticket.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected ticket object")
	}
	currentTicketContent := contentFromBodyJSON(t, ticketObj.BodyJSON)
	expectedTicketContent := "base ticket content\n\nticket update one\n\nticket update two"
	if currentTicketContent != expectedTicketContent {
		t.Fatalf("ticket content mismatch: got %q want %q", currentTicketContent, expectedTicketContent)
	}

	ticketVersions, err := store.ListTicketVersions(ctx, userID, tracker.Slug, ticket.Slug, 10)
	if err != nil {
		t.Fatalf("ListTicketVersions: %v", err)
	}
	if len(ticketVersions) != 2 {
		t.Fatalf("expected 2 ticket versions, got %d", len(ticketVersions))
	}
	if got := contentFromBodyJSON(t, ticketVersions[0].BodyJSON); got != "base ticket content\n\nticket update one" {
		t.Fatalf("unexpected newest ticket version content: %q", got)
	}
	if got := contentFromBodyJSON(t, ticketVersions[1].BodyJSON); got != "base ticket content" {
		t.Fatalf("unexpected oldest ticket version content: %q", got)
	}

	if _, err := store.CreateTicketCommentUpdate(ctx, userID, tracker.Slug, ticket.Slug, comment.Slug, "comment update one"); err != nil {
		t.Fatalf("CreateTicketCommentUpdate one: %v", err)
	}
	if _, err := store.CreateTicketCommentUpdate(ctx, userID, tracker.Slug, ticket.Slug, comment.Slug, "comment update two"); err != nil {
		t.Fatalf("CreateTicketCommentUpdate two: %v", err)
	}

	commentObj, found, err := store.GetLocalTicketCommentObjectBySlug(ctx, tracker.Slug, ticket.Slug, comment.Slug)
	if err != nil {
		t.Fatalf("GetLocalTicketCommentObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected comment object")
	}
	currentCommentContent := contentFromBodyJSON(t, commentObj.BodyJSON)
	expectedCommentContent := "base comment content\n\ncomment update one\n\ncomment update two"
	if currentCommentContent != expectedCommentContent {
		t.Fatalf("comment content mismatch: got %q want %q", currentCommentContent, expectedCommentContent)
	}

	commentVersions, err := store.ListTicketCommentVersions(ctx, userID, tracker.Slug, ticket.Slug, comment.Slug, 10)
	if err != nil {
		t.Fatalf("ListTicketCommentVersions: %v", err)
	}
	if len(commentVersions) != 2 {
		t.Fatalf("expected 2 comment versions, got %d", len(commentVersions))
	}
	if got := contentFromBodyJSON(t, commentVersions[0].BodyJSON); got != "base comment content\n\ncomment update one" {
		t.Fatalf("unexpected newest comment version content: %q", got)
	}
	if got := contentFromBodyJSON(t, commentVersions[1].BodyJSON); got != "base comment content" {
		t.Fatalf("unexpected oldest comment version content: %q", got)
	}
}

func contentFromBodyJSON(t *testing.T, raw []byte) string {
	t.Helper()
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal object body: %v", err)
	}
	value, ok := body["content"]
	if !ok {
		t.Fatalf("object body missing content field")
	}
	content, ok := value.(string)
	if !ok {
		t.Fatalf("object body content has unexpected type %T", value)
	}
	return content
}
