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
	"net/http"
	"net/url"
	"strings"

	"github.com/refinement-systems/BorealValley/src/internal/assets"
	"github.com/refinement-systems/BorealValley/src/internal/common"
)

var objectRepoTmpl = parseWithLayout(assets.HtmlObjectRepo)
var objectTicketTrackerTmpl = parseWithLayout(assets.HtmlObjectTicketTracker)
var objectTicketTmpl = parseWithLayout(assets.HtmlObjectTicket)
var objectTicketCommentTmpl = parseWithLayout(assets.HtmlObjectTicketComment)

type objectRepoTemplateData struct {
	Slug                 string
	ActorID              string
	Name                 string
	TicketTrackerActorID string
	WebURL               string
	ObjectJSON           string
}

type objectTicketTrackerTemplateData struct {
	Slug       string
	ActorID    string
	Name       string
	Summary    string
	WebURL     string
	ObjectJSON string
}

type objectTicketTemplateData struct {
	TicketSlug        string
	TrackerSlug       string
	RepositorySlug    string
	ActorID           string
	TrackerActorID    string
	RepositoryActorID string
	Summary           string
	Content           string
	Published         string
	AttributedTo      string
	AssigneeActionURL string
	Assignees         []objectTicketAssigneeTemplateData
	CommentPostURL    string
	Comments          []objectTicketCommentTemplateData
	Err               string
	WebURL            string
	ObjectJSON        string
}

type objectTicketAssigneeTemplateData struct {
	Username string
	ActorID  string
}

type objectTicketCommentTemplateData struct {
	Slug             string
	ActorID          string
	InReplyTo        string
	InReplyToHref    string
	InReplyToLabel   string
	AttributedTo     string
	To               string
	Content          string
	Published        string
	WebURL           string
	ObjectJSON       string
	CommentActionURL string
}

func wantsActivityPubJSON(accept string) bool {
	accept = strings.ToLower(accept)
	return strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json")
}

func (app *application) objectUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		renderError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	requestedUsername := strings.TrimSpace(r.PathValue("name"))
	if requestedUsername == "" {
		renderNotFound(w)
		return
	}

	record, ok, err := app.store.GetUserActorByUsername(r.Context(), requestedUsername)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok {
		renderNotFound(w)
		return
	}

	actorJSON, err := sanitizeActorJSON(record.ActorJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Vary", "Accept")
	if wantsActivityPubJSON(r.Header.Get("Accept")) {
		writeActivityPubObject(w, actorJSON)
		return
	}

	pretty, err := prettyJSON(actorJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	renderTemplate(w, userCtlTmpl, userCtlTemplateData{
		Username:  record.Username,
		ActorJSON: string(pretty),
	})
}

func (app *application) objectRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		renderError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	repoSlug := strings.TrimSpace(r.PathValue("repo"))
	record, found, err := app.store.GetLocalRepositoryObjectBySlug(r.Context(), repoSlug)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		renderNotFound(w)
		return
	}

	w.Header().Set("Vary", "Accept")
	if wantsActivityPubJSON(r.Header.Get("Accept")) {
		writeActivityPubObject(w, record.BodyJSON)
		return
	}

	body, err := parseObjectBody(record.BodyJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	pretty, err := prettyJSON(record.BodyJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}

	renderTemplate(w, objectRepoTmpl, objectRepoTemplateData{
		Slug:                 repoSlug,
		ActorID:              record.PrimaryKey,
		Name:                 stringField(body, "name"),
		TicketTrackerActorID: stringField(body, "ticketsTrackedBy"),
		WebURL:               "/web/repo/" + url.PathEscape(repoSlug),
		ObjectJSON:           string(pretty),
	})
}

func (app *application) objectTicketTracker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		renderError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	record, found, err := app.store.GetLocalTicketTrackerObjectBySlug(r.Context(), trackerSlug)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		renderNotFound(w)
		return
	}

	w.Header().Set("Vary", "Accept")
	if wantsActivityPubJSON(r.Header.Get("Accept")) {
		writeActivityPubObject(w, record.BodyJSON)
		return
	}

	body, err := parseObjectBody(record.BodyJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	pretty, err := prettyJSON(record.BodyJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}

	renderTemplate(w, objectTicketTrackerTmpl, objectTicketTrackerTemplateData{
		Slug:       trackerSlug,
		ActorID:    record.PrimaryKey,
		Name:       stringField(body, "name"),
		Summary:    stringField(body, "summary"),
		WebURL:     "/web/ticket-tracker/" + url.PathEscape(trackerSlug),
		ObjectJSON: string(pretty),
	})
}

func (app *application) objectTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		renderError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	app.renderTicketObjectPage(w, r, strings.TrimSpace(r.PathValue("tracker")), strings.TrimSpace(r.PathValue("ticket")), "")
}

func (app *application) objectTicketComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		renderError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	commentSlug := strings.TrimSpace(r.PathValue("comment"))
	if trackerSlug == "" || ticketSlug == "" || commentSlug == "" {
		renderNotFound(w)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		renderError(w, http.StatusUnauthorized, "authentication error")
		return
	}

	canAccess, err := app.store.CanAccessTicket(r.Context(), userID, trackerSlug, ticketSlug)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !canAccess {
		renderNotFound(w)
		return
	}

	record, found, err := app.store.GetLocalTicketCommentObjectBySlug(r.Context(), trackerSlug, ticketSlug, commentSlug)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		renderNotFound(w)
		return
	}

	w.Header().Set("Vary", "Accept")
	if wantsActivityPubJSON(r.Header.Get("Accept")) {
		writeActivityPubObject(w, record.BodyJSON)
		return
	}

	body, err := parseObjectBody(record.BodyJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	pretty, err := prettyJSON(record.BodyJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}

	renderTemplate(w, objectTicketCommentTmpl, objectTicketCommentTemplateData{
		Slug:           record.CommentSlug,
		ActorID:        record.PrimaryKey,
		InReplyTo:      stringField(body, "inReplyTo"),
		InReplyToLabel: stringField(body, "inReplyTo"),
		AttributedTo:   stringField(body, "attributedTo"),
		To:             firstStringFromArrayField(body, "to"),
		Content:        stringField(body, "content"),
		Published:      stringField(body, "published"),
		WebURL: "/ticket-tracker/" + url.PathEscape(record.TrackerSlug) + "/ticket/" +
			url.PathEscape(record.TicketSlug) + "#comment-" + url.PathEscape(record.CommentSlug),
		ObjectJSON: string(pretty),
	})
}

func (app *application) renderTicketObjectPage(w http.ResponseWriter, r *http.Request, trackerSlug, ticketSlug, errMsg string) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	ticketSlug = strings.TrimSpace(ticketSlug)
	if trackerSlug == "" || ticketSlug == "" {
		renderNotFound(w)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		renderError(w, http.StatusUnauthorized, "authentication error")
		return
	}

	canAccess, err := app.store.CanAccessTicket(r.Context(), userID, trackerSlug, ticketSlug)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !canAccess {
		renderNotFound(w)
		return
	}

	record, found, err := app.store.GetLocalTicketObjectBySlug(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		renderNotFound(w)
		return
	}

	comments, err := app.store.ListTicketCommentsForTicket(r.Context(), userID, trackerSlug, ticketSlug)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			renderNotFound(w)
			return
		}
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	assignees, err := app.store.ListTicketAssigneesForTicket(r.Context(), trackerSlug, ticketSlug)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Vary", "Accept")
	if wantsActivityPubJSON(r.Header.Get("Accept")) {
		writeActivityPubObject(w, record.BodyJSON)
		return
	}

	body, err := parseObjectBody(record.BodyJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	pretty, err := prettyJSON(record.BodyJSON)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "internal error")
		return
	}

	commentViews := make([]objectTicketCommentTemplateData, 0, len(comments))
	assigneeViews := make([]objectTicketAssigneeTemplateData, 0, len(assignees))
	parentLabels := map[string]string{record.PrimaryKey: "ticket"}
	for _, comment := range comments {
		parentLabels[comment.ActorID] = comment.Slug
	}
	for _, comment := range comments {
		parentLabel := "ticket"
		if !comment.InReplyToTicketID {
			if label, ok := parentLabels[comment.InReplyToActorID]; ok {
				parentLabel = label
			} else if strings.TrimSpace(comment.InReplyToActorID) != "" {
				parentLabel = comment.InReplyToActorID
			}
		}
		inReplyToHref := "#"
		if !comment.InReplyToTicketID {
			if label, ok := parentLabels[comment.InReplyToActorID]; ok {
				inReplyToHref = "#comment-" + label
			}
		}
		commentViews = append(commentViews, objectTicketCommentTemplateData{
			Slug:             comment.Slug,
			ActorID:          comment.ActorID,
			InReplyTo:        comment.InReplyToActorID,
			InReplyToHref:    inReplyToHref,
			InReplyToLabel:   parentLabel,
			AttributedTo:     comment.AttributedTo,
			Content:          comment.Content,
			Published:        comment.Published,
			CommentActionURL: "/web/ticket-tracker/" + url.PathEscape(record.TrackerSlug) + "/ticket/" + url.PathEscape(record.TicketSlug) + "/comment",
		})
	}
	for _, assignee := range assignees {
		assigneeViews = append(assigneeViews, objectTicketAssigneeTemplateData{
			Username: assignee.Username,
			ActorID:  assignee.ActorID,
		})
	}

	renderTemplate(w, objectTicketTmpl, objectTicketTemplateData{
		TicketSlug:        record.TicketSlug,
		TrackerSlug:       record.TrackerSlug,
		RepositorySlug:    record.RepositorySlug,
		ActorID:           record.PrimaryKey,
		TrackerActorID:    stringField(body, "context"),
		RepositoryActorID: stringField(body, "target"),
		Summary:           stringField(body, "summary"),
		Content:           stringField(body, "content"),
		Published:         stringField(body, "published"),
		AttributedTo:      stringField(body, "attributedTo"),
		AssigneeActionURL: "/web/ticket-tracker/" + url.PathEscape(record.TrackerSlug) + "/ticket/" + url.PathEscape(record.TicketSlug) + "/assignee",
		Assignees:         assigneeViews,
		CommentPostURL:    "/web/ticket-tracker/" + url.PathEscape(record.TrackerSlug) + "/ticket/" + url.PathEscape(record.TicketSlug) + "/comment",
		Comments:          commentViews,
		Err:               errMsg,
		WebURL:            "/web/ticket-tracker/" + url.PathEscape(record.TrackerSlug),
		ObjectJSON:        string(pretty),
	})
}

func writeActivityPubObject(w http.ResponseWriter, raw []byte) {
	w.Header().Set("Vary", "Accept")
	w.Header().Set("Content-Type", apMediaType)
	_, _ = w.Write(raw)
}

func parseObjectBody(raw []byte) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	return body, nil
}

func stringField(body map[string]any, key string) string {
	v, _ := body[key].(string)
	return strings.TrimSpace(v)
}

func firstStringFromArrayField(body map[string]any, key string) string {
	raw, ok := body[key]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case []string:
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				return item
			}
		}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}
