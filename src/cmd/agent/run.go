// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL
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
	"log/slog"
	"strings"
	"time"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type runConfig struct {
	StatePath   string
	Workspace   string
	MaxIter     int
	Mode        string
	Model       string
	LMStudioURL string
	CollabMode  string
}

func runAgentOnce(cfg runConfig) error {
	state, err := loadAgentState(cfg.StatePath)
	if err != nil {
		return err
	}

	mode, err := resolveAgentMode(state.Mode)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Mode) != "" {
		mode, err = resolveAgentMode(cfg.Mode)
		if err != nil {
			return err
		}
	}
	state.Mode = mode
	if strings.TrimSpace(cfg.Model) != "" {
		state.Model = strings.TrimSpace(cfg.Model)
	}
	if strings.TrimSpace(cfg.LMStudioURL) != "" {
		state.LMStudioURL = strings.TrimRight(strings.TrimSpace(cfg.LMStudioURL), "/")
	}
	if strings.TrimSpace(state.ServerURL) == "" || strings.TrimSpace(state.ClientID) == "" ||
		strings.TrimSpace(state.ClientSecret) == "" || strings.TrimSpace(state.RedirectURI) == "" {
		return fmt.Errorf("state is incomplete: run init again")
	}
	if state.Mode == agentModeLMStudio &&
		(strings.TrimSpace(state.Model) == "" || strings.TrimSpace(state.LMStudioURL) == "") {
		return fmt.Errorf("state is incomplete: run init again")
	}
	if strings.TrimSpace(state.Token.AccessToken) == "" {
		return fmt.Errorf("missing access token in state: run init again")
	}
	if cfg.MaxIter <= 0 {
		cfg.MaxIter = 3
	}

	state, err = ensureFreshToken(cfg.StatePath, state)
	if err != nil {
		return err
	}

	client := newAPIClient(state.ServerURL, state.Token.AccessToken)
	ticket, ok, err := acquireNextTicket(client)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("no assigned completion-pending tickets")
		return nil
	}

	return processTicket(client, cfg, state, ticket)
}

func ensureFreshToken(statePath string, state agentState) (agentState, error) {
	if !state.Token.ExpiresAt.Before(time.Now().UTC().Add(1 * time.Minute)) {
		return state, nil
	}
	httpClient := defaultHTTPClient()
	meta, err := discoverOAuthMetadata(context.Background(), httpClient, state.ServerURL)
	if err != nil {
		return agentState{}, err
	}
	newToken, err := refreshAccessToken(context.Background(), httpClient, meta.TokenEndpoint, state.ClientID, state.ClientSecret, state.Token)
	if err != nil {
		return agentState{}, err
	}
	state.Token = newToken
	profile, err := fetchProfile(context.Background(), httpClient, state.ServerURL, state.Token.AccessToken)
	if err != nil {
		return agentState{}, err
	}
	state.Profile = profile
	if err := saveAgentState(statePath, state); err != nil {
		return agentState{}, err
	}
	return state, nil
}

func acquireNextTicket(client *apiClient) (common.AssignedTicket, bool, error) {
	tickets, err := client.getAssignedTickets(context.Background(), 1)
	if err != nil {
		return common.AssignedTicket{}, false, err
	}
	if len(tickets) == 0 {
		return common.AssignedTicket{}, false, nil
	}
	return tickets[0], true, nil
}

func processTicket(client *apiClient, cfg runConfig, state agentState, ticket common.AssignedTicket) error {
	slog.Info("processing assigned ticket", "tracker", ticket.TrackerSlug, "ticket", ticket.TicketSlug)

	ackMsg := fmt.Sprintf("Agent acknowledged ticket at %s.", time.Now().UTC().Format(time.RFC3339Nano))
	ackComment, err := client.createTicketComment(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackMsg, common.AgentCommentKindAck)
	if err != nil {
		return err
	}

	repo, err := client.getRepository(context.Background(), ticket.RepositorySlug)
	if err != nil {
		_ = client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "agent_error: "+err.Error())
		return err
	}
	workspace, err := prepareTicketWorkspaceForRun(cfg.Workspace, ticket, repo)
	if err != nil {
		_ = client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "agent_error: "+err.Error())
		return err
	}

	if state.Mode == agentModeTestCounter {
		for i := 1; i <= 3; i++ {
			if err := client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, fmt.Sprintf("test-step:%d", i)); err != nil {
				return err
			}
		}
		if err := finalizeTicketWorkspaceForRun(cfg.Workspace, workspace, ticket); err != nil {
			_ = client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "agent_error: "+err.Error())
			return err
		}
		completionMsg := fmt.Sprintf("Agent completed ticket at %s.", time.Now().UTC().Format(time.RFC3339Nano))
		_, err := client.createTicketComment(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, completionMsg, common.AgentCommentKindCompletion)
		return err
	}

	if err := client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "agent: starting ticket processing"); err != nil {
		return err
	}

	envelope := ticketEnvelope(ticket)
	callbacks := loopCallbacks{
		OnToolCall: func(name, args string) error {
			return client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "tool_call: "+name+" args="+truncateForUpdate(args, 1200))
		},
		OnToolResult: func(name, result string) error {
			return client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "tool_result: "+name+"\n"+truncateForUpdate(result, 2000))
		},
		OnAssistant: func(text string) error {
			return client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "assistant:\n"+truncateForUpdate(text, 4000))
		},
	}

	collabMode, err := resolveCollaborationMode(cfg.CollabMode)
	if err != nil {
		return fmt.Errorf("resolve collab mode: %w", err)
	}
	if collabMode == CollabModePlan {
		callbacks.ApproveToolCall = planModeApprovalFunc()
	}
	answer, err := runLMStudioTicketLoop(context.Background(), state.LMStudioURL, state.Model, workspace.Path, envelope, cfg.MaxIter, collabMode, callbacks)
	if err != nil {
		_ = client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "agent_error: "+err.Error())
		return err
	}
	if err := finalizeTicketWorkspaceForRun(cfg.Workspace, workspace, ticket); err != nil {
		_ = client.createTicketCommentUpdate(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, ackComment.Slug, "agent_error: "+err.Error())
		return err
	}
	completionMsg := fmt.Sprintf("Agent completed ticket at %s.", time.Now().UTC().Format(time.RFC3339Nano))
	if strings.TrimSpace(answer) != "" {
		completionMsg += "\n\n" + strings.TrimSpace(answer)
	}
	_, err = client.createTicketComment(context.Background(), ticket.TrackerSlug, ticket.TicketSlug, completionMsg, common.AgentCommentKindCompletion)
	return err
}

func ticketEnvelope(ticket common.AssignedTicket) string {
	var b strings.Builder
	b.WriteString("TICKET ENVELOPE\n")
	fmt.Fprintf(&b, "id: %s\n", strings.TrimSpace(ticket.TicketSlug))
	fmt.Fprintf(&b, "tracker: %s\n", strings.TrimSpace(ticket.TrackerSlug))
	fmt.Fprintf(&b, "repository: %s\n", strings.TrimSpace(ticket.RepositorySlug))
	fmt.Fprintf(&b, "date: %s\n", ticket.CreatedAt.UTC().Format(time.RFC3339Nano))
	fmt.Fprintf(&b, "priority: %d\n", ticket.Priority)
	fmt.Fprintf(&b, "title: %s\n\n", strings.TrimSpace(ticket.Summary))
	b.WriteString("Description:\n")
	b.WriteString(strings.TrimSpace(ticket.Content))
	b.WriteString("\n")
	return b.String()
}

func truncateForUpdate(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max] + "...(truncated)"
}
