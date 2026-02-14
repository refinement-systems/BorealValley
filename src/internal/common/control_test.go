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

package common

import (
	"testing"
	"time"
)

func TestBuildLocalTicketCommentObjectAgentCommentKind(t *testing.T) {
	t.Parallel()

	published := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	withKind := buildLocalTicketCommentObject(
		"https://example.test/comment/1",
		"https://example.test/ticket/1",
		"https://example.test/ticket/1",
		"https://example.test/users/agent",
		"https://example.test/repo/1",
		"ack",
		AgentCommentKindAck,
		published,
	)
	if got := stringFromAny(withKind[AgentCommentKindField]); got != AgentCommentKindAck {
		t.Fatalf("expected agent comment kind %q, got %q", AgentCommentKindAck, got)
	}

	withoutKind := buildLocalTicketCommentObject(
		"https://example.test/comment/2",
		"https://example.test/ticket/1",
		"https://example.test/ticket/1",
		"https://example.test/users/agent",
		"https://example.test/repo/1",
		"plain",
		"",
		published,
	)
	if _, ok := withoutKind[AgentCommentKindField]; ok {
		t.Fatalf("did not expect %q for plain comment", AgentCommentKindField)
	}
}
