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
	"strings"
	"testing"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

func TestDefaultPrepareTicketWorkspaceForRunRequiresUsablePijulIdentity(t *testing.T) {
	origCheck := hasUsablePijulIdentity
	t.Cleanup(func() { hasUsablePijulIdentity = origCheck })
	hasUsablePijulIdentity = func(context.Context) (bool, error) {
		return false, nil
	}

	_, err := defaultPrepareTicketWorkspaceForRun(
		t.TempDir(),
		common.AssignedTicket{RepositorySlug: "repo-1", TicketSlug: "TCK-1"},
		common.Repository{Slug: "repo-1", Path: "/translated/root/repo/repo-1"},
	)
	if err == nil {
		t.Fatal("expected missing identity error")
	}
	if !strings.Contains(err.Error(), "pijul identity new borealvalley-agent") {
		t.Fatalf("unexpected error: %v", err)
	}
}
