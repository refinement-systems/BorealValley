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
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/refinement-systems/BorealValley/src/internal/assets"
	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type dataListData struct {
	Repositories []common.Repository
	Err          string
}

type dataRepoData struct {
	Repository      common.Repository
	AssignedTracker *common.TicketTracker
	Trackers        []common.TicketTracker
	Members         []common.RepositoryMember
	CanManageMember bool
	Err             string
}

type dataTicketTrackerListData struct {
	Trackers                 []common.TicketTracker
	Err                      string
	CreatedTicketTrackerSlug string
}

type dataTicketTrackerDetailData struct {
	Tracker             common.TicketTracker
	Tickets             []common.Ticket
	TrackedRepositories []common.Repository
	SelectedRepoSlug    string
	CreatedTicketSlug   string
	Err                 string
}

type dataTicketListData struct {
	Tickets []common.Ticket
	Err     string
}

type dataNotificationListData struct {
	Notifications []common.Notification
	Err           string
	NextURL       string
}

var dataListTmpl = template.Must(template.New("data-list").Parse(assets.HtmlDataList))
var dataRepoTmpl = template.Must(template.New("data-project").Parse(assets.HtmlDataProject))
var dataTicketTrackerListTmpl = template.Must(template.New("ticket-tracker-list").Parse(assets.HtmlTicketTrackerList))
var dataTicketTrackerDetailTmpl = template.Must(template.New("ticket-tracker-detail").Parse(assets.HtmlTicketTrackerDetail))
var dataTicketListTmpl = template.Must(template.New("ticket-list").Parse(assets.HtmlTicketList))
var dataNotificationListTmpl = template.Must(template.New("notification-list").Parse(assets.HtmlNotificationList))

func (app *application) dataList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	repositories, err := app.store.ListRepositories(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dataListTmpl.Execute(w, dataListData{Repositories: repositories})
}

func (app *application) dataRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	app.renderRepoPage(w, r, "")
}

func (app *application) dataRepoTicketTracker(w http.ResponseWriter, r *http.Request) {
	repoSlug := strings.TrimSpace(r.PathValue("repo"))
	if repoSlug == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		app.renderRepoPage(w, r, "")
		return
	case http.MethodPost:
		// handled below
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		app.renderRepoPageBySlug(w, r, repoSlug, "bad form")
		return
	}

	action := strings.TrimSpace(r.PostFormValue("action"))
	switch action {
	case "assign":
		trackerSlug := strings.TrimSpace(r.PostFormValue("tracker"))
		if trackerSlug == "" {
			app.renderRepoPageBySlug(w, r, repoSlug, "tracker is required")
			return
		}
		if err := app.store.AssignTicketTrackerToRepository(r.Context(), repoSlug, trackerSlug); err != nil {
			if errors.Is(err, common.ErrValidation) {
				app.renderRepoPageBySlug(w, r, repoSlug, err.Error())
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	case "unassign":
		if err := app.store.UnassignTicketTrackerFromRepository(r.Context(), repoSlug); err != nil {
			if errors.Is(err, common.ErrValidation) {
				app.renderRepoPageBySlug(w, r, repoSlug, err.Error())
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	default:
		app.renderRepoPageBySlug(w, r, repoSlug, "invalid action")
		return
	}

	http.Redirect(w, r, "/web/repo/"+repoSlug, http.StatusSeeOther)
}

func (app *application) dataTicketTrackerList(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		trackers, err := app.store.ListTicketTrackers(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = dataTicketTrackerListTmpl.Execute(w, dataTicketTrackerListData{
			Trackers:                 trackers,
			CreatedTicketTrackerSlug: strings.TrimSpace(r.URL.Query().Get("created")),
		})
		return
	case http.MethodPost:
		// handled below
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		app.renderTicketTrackerListPage(w, r, "bad form")
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	tracker, err := app.store.CreateTicketTracker(
		r.Context(),
		userID,
		strings.TrimSpace(r.PostFormValue("name")),
		strings.TrimSpace(r.PostFormValue("summary")),
	)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			app.renderTicketTrackerListPage(w, r, err.Error())
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/web/ticket-tracker?created="+url.QueryEscape(tracker.Slug), http.StatusSeeOther)
}

func (app *application) dataTicketTrackerDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	app.renderTicketTrackerDetailPage(w, r, strings.TrimSpace(r.PathValue("tracker")), "")
}

func (app *application) dataTicketTrackerTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	if trackerSlug == "" {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		app.renderTicketTrackerDetailPageWithSelection(w, r, trackerSlug, "", "bad form")
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	repoSlug := strings.TrimSpace(r.PostFormValue("repo"))
	priority, err := parseOptionalInt(r.PostFormValue("priority"))
	if err != nil {
		app.renderTicketTrackerDetailPageWithSelection(w, r, trackerSlug, repoSlug, "priority must be a valid integer")
		return
	}
	ticket, err := app.store.CreateTicketWithPriority(
		r.Context(),
		userID,
		trackerSlug,
		repoSlug,
		strings.TrimSpace(r.PostFormValue("summary")),
		strings.TrimSpace(r.PostFormValue("content")),
		priority,
	)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			app.renderTicketTrackerDetailPageWithSelection(w, r, trackerSlug, repoSlug, err.Error())
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	target := fmt.Sprintf("/web/ticket-tracker/%s?repo=%s&created-ticket=%s",
		url.PathEscape(trackerSlug),
		url.QueryEscape(repoSlug),
		url.QueryEscape(ticket.Slug),
	)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (app *application) dataTicketList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	tickets, err := app.store.ListTicketsForUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dataTicketListTmpl.Execute(w, dataTicketListData{Tickets: tickets})
}

func (app *application) dataNotificationList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	minID, _ := parseOptionalPositiveInt64(r.URL.Query().Get("min_id"))
	maxID, _ := parseOptionalPositiveInt64(r.URL.Query().Get("max_id"))
	limit := parseLimitQuery(r.URL.Query().Get("limit"), 20)
	notifications, hasMore, err := app.store.ListNotificationsForUser(r.Context(), userID, common.NotificationListOptions{
		MinID: minID,
		MaxID: maxID,
		Limit: limit,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	nextURL := ""
	if hasMore && len(notifications) > 0 {
		q := url.Values{}
		q.Set("max_id", strconv.FormatInt(notifications[len(notifications)-1].ID, 10))
		q.Set("limit", strconv.Itoa(limit))
		nextURL = "/web/notification?" + q.Encode()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dataNotificationListTmpl.Execute(w, dataNotificationListData{
		Notifications: notifications,
		NextURL:       nextURL,
	})
}

func (app *application) dataNotificationClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}
	if err := app.store.SetAllNotificationsUnread(r.Context(), userID, false); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/web/notification", http.StatusSeeOther)
}

func (app *application) dataNotificationReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}
	if err := app.store.SetAllNotificationsUnread(r.Context(), userID, true); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/web/notification", http.StatusSeeOther)
}

func (app *application) dataNotificationUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}
	notificationID, err := parseRequiredPositiveInt64(r.PathValue("notification"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	unread, err := parseRequiredBool(r.PostFormValue("unread"))
	if err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := app.store.SetNotificationUnread(r.Context(), userID, notificationID, unread); err != nil {
		if errors.Is(err, common.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/web/notification", http.StatusSeeOther)
}

func (app *application) renderRepoPage(w http.ResponseWriter, r *http.Request, errMsg string) {
	repoSlug := strings.TrimSpace(r.PathValue("repo"))
	if repoSlug == "" {
		http.NotFound(w, r)
		return
	}
	app.renderRepoPageBySlug(w, r, repoSlug, errMsg)
}

func (app *application) renderRepoPageBySlug(w http.ResponseWriter, r *http.Request, repoSlug string, errMsg string) {
	repo, found, err := app.repositoryFromPathValue(r.Context(), repoSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	trackers, err := app.store.ListTicketTrackers(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	members, err := app.store.ListRepositoryMembers(r.Context(), repoSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	isAdmin, err := sessionUserIsAdmin(app, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var assigned *common.TicketTracker
	if repo.TicketTrackerSlug != "" {
		for i := range trackers {
			if trackers[i].Slug == repo.TicketTrackerSlug {
				assigned = &trackers[i]
				break
			}
		}
		if assigned == nil {
			tracker, found, err := app.store.GetTicketTrackerBySlug(r.Context(), repo.TicketTrackerSlug)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if found {
				assigned = &tracker
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dataRepoTmpl.Execute(w, dataRepoData{
		Repository:      repo,
		AssignedTracker: assigned,
		Trackers:        trackers,
		Members:         members,
		CanManageMember: isAdmin,
		Err:             errMsg,
	})
}

func (app *application) renderTicketTrackerListPage(w http.ResponseWriter, r *http.Request, errMsg string) {
	trackers, err := app.store.ListTicketTrackers(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dataTicketTrackerListTmpl.Execute(w, dataTicketTrackerListData{
		Trackers: trackers,
		Err:      errMsg,
	})
}

func (app *application) renderTicketTrackerDetailPage(w http.ResponseWriter, r *http.Request, trackerSlug string, errMsg string) {
	app.renderTicketTrackerDetailPageWithSelection(w, r, trackerSlug, strings.TrimSpace(r.URL.Query().Get("repo")), errMsg)
}

func (app *application) renderTicketTrackerDetailPageWithSelection(w http.ResponseWriter, r *http.Request, trackerSlug string, selectedRepoSlug string, errMsg string) {
	tracker, found, err := app.ticketTrackerFromPathValue(r.Context(), trackerSlug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	repositories, err := app.store.ListRepositoriesForTracker(r.Context(), tracker.Slug)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	tickets, err := app.store.ListTicketsForTrackerForUser(r.Context(), tracker.Slug, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	selectedRepoSlug = strings.TrimSpace(selectedRepoSlug)
	if selectedRepoSlug == "" && len(repositories) > 0 {
		selectedRepoSlug = repositories[0].Slug
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dataTicketTrackerDetailTmpl.Execute(w, dataTicketTrackerDetailData{
		Tracker:             tracker,
		Tickets:             tickets,
		TrackedRepositories: repositories,
		SelectedRepoSlug:    selectedRepoSlug,
		CreatedTicketSlug:   strings.TrimSpace(r.URL.Query().Get("created-ticket")),
		Err:                 errMsg,
	})
}

func (app *application) repositoryFromPathValue(ctx context.Context, slug string) (common.Repository, bool, error) {
	return app.store.GetRepositoryBySlug(ctx, strings.TrimSpace(slug))
}

func (app *application) ticketTrackerFromPathValue(ctx context.Context, slug string) (common.TicketTracker, bool, error) {
	return app.store.GetTicketTrackerBySlug(ctx, strings.TrimSpace(slug))
}

func (app *application) dataRepoMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	repoSlug := strings.TrimSpace(r.PathValue("repo"))
	if repoSlug == "" {
		http.NotFound(w, r)
		return
	}

	isAdmin, err := sessionUserIsAdmin(app, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		http.Error(w, "permission error", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		app.renderRepoPageBySlug(w, r, repoSlug, "bad form")
		return
	}

	action := strings.TrimSpace(r.PostFormValue("action"))
	username := strings.TrimSpace(r.PostFormValue("username"))
	if username == "" {
		app.renderRepoPageBySlug(w, r, repoSlug, "username is required")
		return
	}

	switch action {
	case "add":
		err = app.store.AddRepositoryMemberByUsername(r.Context(), repoSlug, username)
	case "remove":
		err = app.store.RemoveRepositoryMemberByUsername(r.Context(), repoSlug, username)
	default:
		app.renderRepoPageBySlug(w, r, repoSlug, "invalid action")
		return
	}
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			app.renderRepoPageBySlug(w, r, repoSlug, err.Error())
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/web/repo/"+repoSlug, http.StatusSeeOther)
}

func (app *application) dataTicketComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	if trackerSlug == "" || ticketSlug == "" {
		http.NotFound(w, r)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		app.renderTicketObjectPage(w, r, trackerSlug, ticketSlug, "bad form")
		return
	}

	comment, err := app.store.CreateTicketComment(
		r.Context(),
		userID,
		trackerSlug,
		ticketSlug,
		strings.TrimSpace(r.PostFormValue("content")),
		strings.TrimSpace(r.PostFormValue("in_reply_to")),
	)
	if err != nil {
		if errors.Is(err, common.ErrValidation) {
			app.renderTicketObjectPage(w, r, trackerSlug, ticketSlug, err.Error())
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(
		w,
		r,
		"/ticket-tracker/"+url.PathEscape(trackerSlug)+"/ticket/"+url.PathEscape(ticketSlug)+"#comment-"+url.PathEscape(comment.Slug),
		http.StatusSeeOther,
	)
}

func (app *application) dataTicketAssignee(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	trackerSlug := strings.TrimSpace(r.PathValue("tracker"))
	ticketSlug := strings.TrimSpace(r.PathValue("ticket"))
	if trackerSlug == "" || ticketSlug == "" {
		http.NotFound(w, r)
		return
	}

	userID, ok := sessionUserIDFromContext(app, r)
	if !ok {
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		app.renderTicketObjectPage(w, r, trackerSlug, ticketSlug, "bad form")
		return
	}
	action := strings.TrimSpace(r.PostFormValue("action"))
	username := strings.TrimSpace(r.PostFormValue("username"))

	if err := app.store.UpdateTicketAssigneeByUsername(r.Context(), userID, trackerSlug, ticketSlug, action, username); err != nil {
		if errors.Is(err, common.ErrValidation) {
			app.renderTicketObjectPage(w, r, trackerSlug, ticketSlug, err.Error())
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(
		w,
		r,
		"/ticket-tracker/"+url.PathEscape(trackerSlug)+"/ticket/"+url.PathEscape(ticketSlug),
		http.StatusSeeOther,
	)
}

func parseRequiredBool(raw string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, errors.New("missing bool")
	}
	return strconv.ParseBool(raw)
}

func parseOptionalInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}
