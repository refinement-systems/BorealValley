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
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

func TestTicketTrackerDetailTemplateShowsExistingTickets(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := dataTicketTrackerDetailTmpl.Execute(&body, map[string]any{
		"Tracker": common.TicketTracker{Slug: "tracker-a", Name: "Tracker A"},
		"TrackedRepositories": []common.Repository{
			{Slug: "repo-a"},
		},
		"SelectedRepoSlug": "repo-a",
		"Tickets": []common.Ticket{
			{
				Slug:           "ticket-001",
				ActorID:        "https://example.test/ticket-tracker/tracker-a/ticket/ticket-001",
				TrackerSlug:    "tracker-a",
				RepositorySlug: "repo-a",
				Summary:        "Fix login bug",
			},
		},
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}

	out := body.String()
	if !strings.Contains(out, "Existing Tickets") {
		t.Fatalf("expected existing tickets section, got:\n%s", out)
	}
	if !strings.Contains(out, `<a href="/ticket-tracker/tracker-a/ticket/ticket-001">Fix login bug</a>`) {
		t.Fatalf("expected linked ticket summary in output, got:\n%s", out)
	}
	if !strings.Contains(out, `name="priority"`) {
		t.Fatalf("expected priority input in create ticket form, got:\n%s", out)
	}
}

func TestTicketListTemplateRendersTicketRows(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := dataTicketListTmpl.Execute(&body, dataTicketListData{
		Tickets: []common.Ticket{
			{
				Slug:           "ticket-002",
				ActorID:        "https://example.test/ticket-tracker/tracker-b/ticket/ticket-002",
				TrackerSlug:    "tracker-b",
				RepositorySlug: "repo-b",
				Summary:        "Dashboard crash",
			},
		},
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}

	out := body.String()
	if !strings.Contains(out, `<th>Name</th>`) {
		t.Fatalf("expected Name header, got:\n%s", out)
	}
	if !strings.Contains(out, `<a href="/ticket-tracker/tracker-b/ticket/ticket-002">Dashboard crash</a>`) {
		t.Fatalf("expected linked ticket summary in output, got:\n%s", out)
	}
	if !strings.Contains(out, `<td>tracker-b</td>`) {
		t.Fatalf("expected tracker column value, got:\n%s", out)
	}
	if !strings.Contains(out, `<td>repo-b</td>`) {
		t.Fatalf("expected repository column value, got:\n%s", out)
	}
}

func TestNotificationListTemplateRendersRows(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := dataNotificationListTmpl.Execute(&body, dataNotificationListData{
		Notifications: []common.Notification{
			{
				ID:             12,
				Type:           "ticket_assigned",
				Unread:         true,
				CreatedAt:      time.Unix(1700000000, 0).UTC(),
				RepositorySlug: "repo-a",
				TrackerSlug:    "tracker-a",
				TicketSlug:     "ticket-001",
				Account: common.NotificationAccount{
					ID:       7,
					Username: "alice",
					ActorID:  "https://example.test/users/alice",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}

	out := body.String()
	if !strings.Contains(out, "ticket_assigned") {
		t.Fatalf("expected notification type, got:\n%s", out)
	}
	if !strings.Contains(out, "alice") {
		t.Fatalf("expected account username, got:\n%s", out)
	}
	if !strings.Contains(out, `name="unread"`) {
		t.Fatalf("expected unread controls, got:\n%s", out)
	}
}
