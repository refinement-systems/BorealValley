# Agent Completion Comments Design

Date: 2026-03-17

## Overview

Adjust `BorealValley-agent run` so the agent still posts one acknowledgement comment immediately, keeps tool-call and assistant progress as ticket `Update` objects, and posts one final completion comment only after work is done.

Ticket eligibility for the agent must no longer be driven by the generic "has this user ever commented?" rule, because that creates the current ack-first crash gap. Instead, the agent will fetch assigned tickets that do not yet have an agent completion comment from that same user.

## Goals

- Preserve the current update stream for progress and error visibility.
- Add an explicit, durable completion marker as a ticket comment.
- Prevent acknowledgement-only runs from causing tickets to disappear from future agent runs.
- Avoid changing existing human assignee semantics for the generic assigned-ticket API.

## Non-Goals

- No new database tables.
- No migration framework changes.
- No reply-thread model for agent progress in this iteration.
- No attempt to deduplicate repeated acknowledgement comments after a crash/retry.

## Comment Metadata

Ticket comments already store arbitrary JSON in `as_note.body`, so agent-only metadata can live there without schema changes.

Add an optional extension field to local ticket comments:

- `borealValleyAgentCommentKind`

Allowed agent values for now:

- `ack`
- `completion`

Regular user comments omit the field.

## Assigned-Ticket Query Semantics

Keep the current `UnrespondedOnly` behavior unchanged:

- `responded_by_me` remains true when the current user has any comment on the ticket.
- `unresponded=true` still excludes tickets where the current user has commented.

Add a separate agent-specific filter to `ListAssignedTicketsForUser` and `GET /api/v1/ticket/assigned`:

- request flag: `agent_completion_pending=true`
- effect: include only tickets that do not already have a ticket comment by the current user with `borealValleyAgentCommentKind = "completion"`

This isolates the agent lifecycle from the generic human "responded" semantics.

## Agent Run Flow

New `agent run` flow:

1. Fetch one assigned ticket with `agent_completion_pending=true`.
2. Post acknowledgement comment tagged `ack`.
3. Post progress as ticket updates:
   - start marker
   - tool calls
   - tool results
   - assistant output
   - terminal error
4. On successful completion, post one root completion comment tagged `completion`.

If the process crashes after the acknowledgement comment but before the completion comment, the ticket remains eligible for future runs.

## Error Handling

- Failures during LM Studio execution still publish `agent_error:` ticket updates.
- No completion comment is posted on failure.
- A rerun may produce a second acknowledgement comment before eventual success. This is acceptable in this iteration because it preserves correctness without introducing run-state coordination.

## Testing

Add or update tests to cover:

- comment metadata persistence for agent comment kinds
- assigned-ticket filtering with `agent_completion_pending=true`
- agent run posting `ack` then updates then `completion`
- failure path posting no completion comment
- e2e path showing both comments and updates, with the ticket excluded only after completion

