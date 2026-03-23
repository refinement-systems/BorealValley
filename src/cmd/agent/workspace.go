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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type ticketWorkspace struct {
	Path              string
	SourceRepoPath    string
	BaselineUntracked map[string]struct{}
}

var prepareTicketWorkspaceForRun = defaultPrepareTicketWorkspaceForRun
var finalizeTicketWorkspaceForRun = defaultFinalizeTicketWorkspaceForRun

func defaultPrepareTicketWorkspaceForRun(parent string, ticket common.AssignedTicket, repo common.Repository) (ticketWorkspace, error) {
	parent = strings.TrimSpace(parent)
	sourceRepoPath := strings.TrimSpace(repo.Path)
	if parent == "" {
		return ticketWorkspace{}, fmt.Errorf("workspace required")
	}
	if sourceRepoPath == "" {
		return ticketWorkspace{}, fmt.Errorf("repository path missing for %s", ticket.RepositorySlug)
	}

	checkoutPath := filepath.Join(parent, ticket.RepositorySlug, ticket.TicketSlug)
	if err := os.RemoveAll(checkoutPath); err != nil {
		return ticketWorkspace{}, err
	}
	if err := os.MkdirAll(filepath.Dir(checkoutPath), 0o755); err != nil {
		return ticketWorkspace{}, err
	}
	if err := common.ClonePijulRepo(context.Background(), sourceRepoPath, checkoutPath); err != nil {
		return ticketWorkspace{}, err
	}
	baselineUntracked, err := common.SnapshotUntrackedPaths(context.Background(), checkoutPath)
	if err != nil {
		return ticketWorkspace{}, err
	}
	return ticketWorkspace{
		Path:              checkoutPath,
		SourceRepoPath:    sourceRepoPath,
		BaselineUntracked: baselineUntracked,
	}, nil
}

func defaultFinalizeTicketWorkspaceForRun(_ string, workspace ticketWorkspace, ticket common.AssignedTicket) error {
	_, err := common.CommitPijulChanges(context.Background(), workspace.Path, workspace.BaselineUntracked, ticketCommitMessage(ticket))
	return err
}

func ticketCommitMessage(ticket common.AssignedTicket) string {
	summary := strings.TrimSpace(ticket.Summary)
	if summary == "" {
		return strings.TrimSpace(ticket.TicketSlug)
	}
	return strings.TrimSpace(ticket.TicketSlug) + ": " + summary
}
