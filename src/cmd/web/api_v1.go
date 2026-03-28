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
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

func (app *application) apiV1Profile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	profile, found, err := app.store.GetOAuthUserProfile(r.Context(), principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          profile.UserID,
		"username":    profile.Username,
		"actor_id":    profile.ActorID,
		"main_key_id": profile.MainKeyID,
	})
}

func (app *application) apiV1RepoList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repos, err := app.store.ListRepositories(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repo": mapRepositories(app.repoPathMapper, repos)})
}

func (app *application) apiV1RepoDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo, found, err := app.store.GetRepositoryBySlug(r.Context(), strings.TrimSpace(r.PathValue("repo")))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, mapRepository(app.repoPathMapper, repo))
}

func (app *application) apiV1TicketTrackerList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	trackers, err := app.store.ListTicketTrackers(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket_tracker": trackers})
}

func (app *application) apiV1TicketTrackerCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	var body struct {
		Name    string `json:"name"`
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	tracker, err := app.store.CreateTicketTracker(r.Context(), principal.UserID, body.Name, body.Summary)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, tracker)
}

func (app *application) apiV1TicketTrackerDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tracker, found, err := app.store.GetTicketTrackerBySlug(r.Context(), strings.TrimSpace(r.PathValue("tracker")))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, tracker)
}

func (app *application) apiV1RepoTicketTrackerAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	repoSlug := strings.TrimSpace(r.PathValue("repo"))
	if repoSlug == "" {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessRepository(r.Context(), repoSlug, principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}
	var body struct {
		Action  string `json:"action"`
		Tracker string `json:"tracker"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(body.Action)
	trackerSlug := strings.TrimSpace(body.Tracker)
	if action == "" {
		action = "assign"
	}
	switch action {
	case "assign":
		err = app.store.AssignTicketTrackerToRepository(r.Context(), repoSlug, trackerSlug)
	case "unassign":
		err = app.store.UnassignTicketTrackerFromRepository(r.Context(), repoSlug)
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
		return
	}
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (app *application) apiV1TicketList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	tickets, err := app.store.ListTicketsForTrackerForUser(r.Context(), trackerSlug, principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": tickets})
}

func (app *application) apiV1TicketAssignedList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	unrespondedOnly, err := parseOptionalBoolQuery(r.URL.Query().Get("unresponded"), "unresponded")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	agentCompletionPendingOnly, err := parseOptionalBoolQuery(r.URL.Query().Get("agent_completion_pending"), "agent_completion_pending")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	limit := parseLimitQuery(r.URL.Query().Get("limit"), 20)
	tickets, err := app.store.ListAssignedTicketsForUser(r.Context(), principal.UserID, common.AssignedTicketListOptions{
		Limit:                      limit,
		UnrespondedOnly:            unrespondedOnly,
		AgentCompletionPendingOnly: agentCompletionPendingOnly,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": tickets})
}

func (app *application) apiV1TicketCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	if trackerSlug == "" {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Repository string `json:"repo"`
		Summary    string `json:"summary"`
		Content    string `json:"content"`
		Priority   int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	ticket, err := app.store.CreateTicketWithPriority(r.Context(), principal.UserID, trackerSlug, body.Repository, body.Summary, body.Content, body.Priority)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, ticket)
}

func (app *application) apiV1TicketUpdateCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	record, found, err := app.store.GetLocalTicketObjectBySlug(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessRepository(r.Context(), record.RepositorySlug, principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	update, err := app.store.CreateTicketUpdate(r.Context(), principal.UserID, trackerSlug, ticketSlug, body.Content)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, update)
}

func (app *application) apiV1TicketCommentUpdateCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	commentSlug := strings.TrimSpace(r.PathValue("comment"))

	record, found, err := app.store.GetLocalTicketCommentObjectBySlug(r.Context(), trackerSlug, ticketSlug, commentSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessTicket(r.Context(), principal.UserID, trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}
	_ = record

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	update, err := app.store.CreateTicketCommentUpdate(r.Context(), principal.UserID, trackerSlug, ticketSlug, commentSlug, body.Content)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, update)
}

type apiV1ObjectVersion struct {
	ID                     int64           `json:"id"`
	ObjectPrimaryKey       string          `json:"object_primary_key"`
	ObjectType             string          `json:"object_type"`
	Body                   json.RawMessage `json:"body"`
	SourceUpdatePrimaryKey string          `json:"source_update_primary_key,omitempty"`
	CreatedByUserID        int64           `json:"created_by_user_id,omitempty"`
	CreatedAt              string          `json:"created_at"`
}

func (app *application) apiV1TicketVersionList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	record, found, err := app.store.GetLocalTicketObjectBySlug(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessRepository(r.Context(), record.RepositorySlug, principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}
	limit := parseLimitQuery(r.URL.Query().Get("limit"), 20)
	versions, err := app.store.ListTicketVersions(r.Context(), principal.UserID, trackerSlug, ticketSlug, limit)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]apiV1ObjectVersion, 0, len(versions))
	for _, v := range versions {
		out = append(out, apiV1ObjectVersion{
			ID:                     v.ID,
			ObjectPrimaryKey:       v.ObjectPrimaryKey,
			ObjectType:             v.ObjectType,
			Body:                   json.RawMessage(v.BodyJSON),
			SourceUpdatePrimaryKey: v.SourceUpdatePrimaryKey,
			CreatedByUserID:        v.CreatedByUserID,
			CreatedAt:              v.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": out})
}

func (app *application) apiV1TicketCommentVersionList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	commentSlug := strings.TrimSpace(r.PathValue("comment"))
	record, found, err := app.store.GetLocalTicketCommentObjectBySlug(r.Context(), trackerSlug, ticketSlug, commentSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessTicket(r.Context(), principal.UserID, trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}
	_ = record
	limit := parseLimitQuery(r.URL.Query().Get("limit"), 20)
	versions, err := app.store.ListTicketCommentVersions(r.Context(), principal.UserID, trackerSlug, ticketSlug, commentSlug, limit)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]apiV1ObjectVersion, 0, len(versions))
	for _, v := range versions {
		out = append(out, apiV1ObjectVersion{
			ID:                     v.ID,
			ObjectPrimaryKey:       v.ObjectPrimaryKey,
			ObjectType:             v.ObjectType,
			Body:                   json.RawMessage(v.BodyJSON),
			SourceUpdatePrimaryKey: v.SourceUpdatePrimaryKey,
			CreatedByUserID:        v.CreatedByUserID,
			CreatedAt:              v.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": out})
}

func (app *application) apiV1TicketCommentList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	record, found, err := app.store.GetLocalTicketObjectBySlug(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessRepository(r.Context(), record.RepositorySlug, principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	comments, err := app.store.ListTicketCommentsForTicket(r.Context(), principal.UserID, trackerSlug, ticketSlug)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comment": comments})
}

func (app *application) apiV1TicketCommentCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	record, found, err := app.store.GetLocalTicketObjectBySlug(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessRepository(r.Context(), record.RepositorySlug, principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	var body struct {
		Content          string `json:"content"`
		InReplyTo        string `json:"in_reply_to"`
		AgentCommentKind string `json:"agent_comment_kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	var comment common.TicketComment
	if strings.TrimSpace(body.AgentCommentKind) == "" {
		comment, err = app.store.CreateTicketComment(
			r.Context(),
			principal.UserID,
			trackerSlug,
			ticketSlug,
			body.Content,
			body.InReplyTo,
		)
	} else {
		comment, err = app.store.CreateAgentTicketComment(
			r.Context(),
			principal.UserID,
			trackerSlug,
			ticketSlug,
			body.Content,
			body.InReplyTo,
			body.AgentCommentKind,
		)
	}
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, comment)
}

func parseOptionalBoolQuery(raw, field string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid %s", strings.TrimSpace(field))
	}
	return parsed, nil
}

func (app *application) apiV1TicketAssigneeList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	record, found, err := app.store.GetLocalTicketObjectBySlug(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessRepository(r.Context(), record.RepositorySlug, principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	assignees, err := app.store.ListTicketAssigneesForTicket(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assignee": assignees})
}

func (app *application) apiV1TicketAssigneeUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	record, found, err := app.store.GetLocalTicketObjectBySlug(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canAccess, err := app.store.CanAccessRepository(r.Context(), record.RepositorySlug, principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	var body struct {
		Action   string `json:"action"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if err := app.store.UpdateTicketAssigneeByUsername(
		r.Context(),
		principal.UserID,
		trackerSlug,
		ticketSlug,
		body.Action,
		body.Username,
	); err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

type apiV1NotificationAccount struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	ActorID  string `json:"actor_id"`
}

type apiV1NotificationTicket struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Tracker string `json:"tracker"`
	Repo    string `json:"repo"`
}

type apiV1Notification struct {
	ID        string                   `json:"id"`
	Type      string                   `json:"type"`
	Unread    bool                     `json:"unread"`
	CreatedAt string                   `json:"created_at"`
	Account   apiV1NotificationAccount `json:"account"`
	Ticket    apiV1NotificationTicket  `json:"ticket"`
}

func (app *application) apiV1NotificationList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	minID, err := parseOptionalPositiveInt64(r.URL.Query().Get("min_id"))
	if err != nil {
		http.Error(w, "invalid min_id", http.StatusBadRequest)
		return
	}
	maxID, err := parseOptionalPositiveInt64(r.URL.Query().Get("max_id"))
	if err != nil {
		http.Error(w, "invalid max_id", http.StatusBadRequest)
		return
	}
	limit := parseLimitQuery(r.URL.Query().Get("limit"), 20)
	notifications, hasMore, err := app.store.ListNotificationsForUser(r.Context(), principal.UserID, common.NotificationListOptions{
		MinID: minID,
		MaxID: maxID,
		Limit: limit,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	out := make([]apiV1Notification, 0, len(notifications))
	for _, item := range notifications {
		accountID := ""
		if item.Account.ID > 0 {
			accountID = strconv.FormatInt(item.Account.ID, 10)
		}
		out = append(out, apiV1Notification{
			ID:        strconv.FormatInt(item.ID, 10),
			Type:      item.Type,
			Unread:    item.Unread,
			CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
			Account: apiV1NotificationAccount{
				ID:       accountID,
				Username: item.Account.Username,
				ActorID:  item.Account.ActorID,
			},
			Ticket: apiV1NotificationTicket{
				ID:      item.TicketActorID,
				Slug:    item.TicketSlug,
				Tracker: item.TrackerSlug,
				Repo:    item.RepositorySlug,
			},
		})
	}
	if hasMore && len(notifications) > 0 {
		if link := buildNotificationNextLink(r, notifications[len(notifications)-1].ID, limit); link != "" {
			w.Header().Set("Link", link)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (app *application) apiV1NotificationClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	if err := app.store.SetAllNotificationsUnread(r.Context(), principal.UserID, false); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (app *application) apiV1NotificationReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	if err := app.store.SetAllNotificationsUnread(r.Context(), principal.UserID, true); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (app *application) apiV1NotificationUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}

	notificationID, err := parseRequiredPositiveInt64(r.PathValue("notification"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Unread *bool `json:"unread"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if body.Unread == nil {
		http.Error(w, "unread is required", http.StatusBadRequest)
		return
	}
	if err := app.store.SetNotificationUnread(r.Context(), principal.UserID, notificationID, *body.Unread); err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (app *application) apiV1RepoMemberList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	isAdmin, err := app.store.IsUserAdmin(r.Context(), principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	repoSlug := strings.TrimSpace(r.PathValue("repo"))
	_, found, err := app.store.GetRepositoryBySlug(r.Context(), repoSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	member, err := app.store.ListRepositoryMembers(r.Context(), repoSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": member})
}

func (app *application) apiV1RepoMemberUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := oauthPrincipalFromContext(r.Context())
	if !ok {
		writeBearerUnauthorized(w)
		return
	}
	isAdmin, err := app.store.IsUserAdmin(r.Context(), principal.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	repoSlug := strings.TrimSpace(r.PathValue("repo"))
	_, found, err := app.store.GetRepositoryBySlug(r.Context(), repoSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	var body struct {
		Action   string `json:"action"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	action := strings.TrimSpace(body.Action)
	username := strings.TrimSpace(body.Username)
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	if action == "" {
		action = "add"
	}

	switch action {
	case "add":
		err = app.store.AddRepositoryMemberByUsername(r.Context(), repoSlug, username)
	case "remove":
		err = app.store.RemoveRepositoryMemberByUsername(r.Context(), repoSlug, username)
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
		return
	}
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (app *application) apiV1ObjectCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	counts, err := app.store.ListObjectTypeCounts(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object_count": counts})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
