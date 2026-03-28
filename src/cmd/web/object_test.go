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
)

func TestWantsActivityPubJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		accept string
		want   bool
	}{
		{name: "activity json", accept: "application/activity+json", want: true},
		{name: "ld json", accept: "application/ld+json", want: true},
		{name: "ld json with profile", accept: `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`, want: true},
		{name: "generic json is not enough", accept: "application/json", want: false},
		{name: "browser html", accept: "text/html,application/xhtml+xml", want: false},
		{name: "case insensitive", accept: "Application/Activity+JSON", want: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := wantsActivityPubJSON(tc.accept); got != tc.want {
				t.Fatalf("wantsActivityPubJSON(%q) = %v, want %v", tc.accept, got, tc.want)
			}
		})
	}
}

func TestObjectTicketTemplateRendersCommentAndReplyForms(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := objectTicketTmpl.Execute(&body, objectTicketTemplateData{
		TicketSlug:     "ticket-001",
		TrackerSlug:    "tracker-a",
		RepositorySlug: "repo-a",
		Summary:        "Fix login bug",
		CommentPostURL:    "/web/ticket-tracker/tracker-a/ticket/ticket-001/comment",
		AssigneeActionURL: "/web/ticket-tracker/tracker-a/ticket/ticket-001/assignee",
		Content:           "line one\nline two",
		Assignees: []objectTicketAssigneeTemplateData{
			{
				Username: "alice",
				ActorID:  "https://example.test/users/alice",
			},
		},
		Comments: []objectTicketCommentTemplateData{
			{
				Slug:             "comment-001",
				ActorID:          "https://example.test/ticket-tracker/tracker-a/ticket/ticket-001/comment/comment-001",
				InReplyTo:        "https://example.test/ticket-tracker/tracker-a/ticket/ticket-001",
				InReplyToHref:    "#",
				InReplyToLabel:   "ticket",
				Content:          "hello\nworld",
				CommentActionURL: "/web/ticket-tracker/tracker-a/ticket/ticket-001/comment",
			},
		},
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}

	out := body.String()
	if !strings.Contains(out, `<title>Fix login bug</title>`) {
		t.Fatalf("expected summary in page title, got:\n%s", out)
	}
	if !strings.Contains(out, `<h1 class="page-title">Fix login bug</h1>`) {
		t.Fatalf("expected summary in page heading, got:\n%s", out)
	}
	if !strings.Contains(out, `<a href="/web/ticket-tracker/tracker-a">tracker-a</a>`) {
		t.Fatalf("expected relative tracker link, got:\n%s", out)
	}
	if !strings.Contains(out, `<a href="/web/repo/repo-a">repo-a</a>`) {
		t.Fatalf("expected relative repo link, got:\n%s", out)
	}
	if !strings.Contains(out, `<a href="#">ticket</a>`) {
		t.Fatalf("expected in-reply-to anchor link, got:\n%s", out)
	}
	if !strings.Contains(out, `action="/web/ticket-tracker/tracker-a/ticket/ticket-001/comment"`) {
		t.Fatalf("expected comment action url, got:\n%s", out)
	}
	if !strings.Contains(out, `name="in_reply_to"`) {
		t.Fatalf("expected reply hidden in_reply_to field, got:\n%s", out)
	}
	if !strings.Contains(out, `comment/comment-001`) {
		t.Fatalf("expected canonical comment link, got:\n%s", out)
	}
	if !strings.Contains(out, `name="username"`) {
		t.Fatalf("expected assignee username field, got:\n%s", out)
	}
	if !strings.Contains(out, `ticket/ticket-001/assignee`) {
		t.Fatalf("expected assignee action url, got:\n%s", out)
	}
	if !strings.Contains(out, `class="content-block ticket-description"`) || !strings.Contains(out, `class="content-block comment-content"`) {
		t.Fatalf("expected styled content blocks for ticket and comment content, got:\n%s", out)
	}
	if !strings.Contains(out, "line one\nline two") {
		t.Fatalf("expected ticket content newlines to be preserved in template output, got:\n%s", out)
	}
	if !strings.Contains(out, "hello\nworld") {
		t.Fatalf("expected comment content newlines to be preserved in template output, got:\n%s", out)
	}
}

func TestObjectTicketCommentTemplatePreservesVisibleNewlines(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := objectTicketCommentTmpl.Execute(&body, objectTicketCommentTemplateData{
		Slug:    "comment-001",
		Content: "first line\nsecond line",
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}

	out := body.String()
	if !strings.Contains(out, `class="content-block"`) {
		t.Fatalf("expected styled content block in canonical comment template, got:\n%s", out)
	}
	if !strings.Contains(out, "first line\nsecond line") {
		t.Fatalf("expected canonical comment template to preserve newline text, got:\n%s", out)
	}
}
