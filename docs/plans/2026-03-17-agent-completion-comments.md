# Agent Completion Comments Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make agent runs post a final completion comment and use that explicit marker, rather than any prior comment, to decide whether a ticket is still pending for the agent.

**Architecture:** Keep progress/errors as ticket `Update` objects, add optional agent comment metadata on `Note` bodies, and extend assigned-ticket selection with an agent-only completion filter. The generic human `unresponded` behavior stays unchanged.

**Tech Stack:** Go stdlib, existing PostgreSQL queries in `src/internal/common`, existing web API in `src/cmd/web`, existing agent client/run loop in `src/cmd/agent`.

---

### Task 1: Document the approved design

**Files:**
- Create: `docs/plans/2026-03-17-agent-completion-comments-design.md`
- Create: `docs/plans/2026-03-17-agent-completion-comments.md`

**Step 1: Write the design doc**

Capture:

- agent ack vs completion comment metadata
- agent-specific pending filter
- unchanged human `unresponded` semantics
- crash-gap resolution via completion marker

**Step 2: Write the implementation plan**

List the code, tests, and docs to change, plus verification commands.

**Step 3: Commit**

```bash
git add docs/plans/2026-03-17-agent-completion-comments-design.md docs/plans/2026-03-17-agent-completion-comments.md
git commit -m "plan agent completion comments"
```

---

### Task 2: Add failing tests for comment metadata and pending-agent filtering

**Files:**
- Modify: `src/internal/common/control_integration_test.go`

**Step 1: Write a failing test for agent comment metadata**

Cover:

- creating an agent acknowledgement comment stores `borealValleyAgentCommentKind=ack`
- creating an agent completion comment stores `borealValleyAgentCommentKind=completion`
- plain user comments omit the field

**Step 2: Write a failing test for assigned-ticket filtering**

Cover:

- plain comments still set `responded_by_me=true`
- `UnrespondedOnly` behavior remains unchanged
- new `AgentCompletionPendingOnly` behavior excludes only tickets with completion comments by the current user

**Step 3: Run the targeted test and confirm it fails**

```bash
env GOCACHE=/tmp/bv-gocache go test ./src/internal/common -run 'TestIntegration(CreateTicketCommentStoresAgentCommentKind|ListAssignedTicketsForUserAgentCompletionPending)'
```

**Step 4: Implement the minimal storage/query changes**

Touch:

- `src/internal/common/control.go`
- `src/internal/common/oauth.go`

Add:

- optional comment-kind argument or helper for agent comments
- new assigned-ticket option for pending agent completion

**Step 5: Re-run the targeted test**

```bash
env GOCACHE=/tmp/bv-gocache go test ./src/internal/common -run 'TestIntegration(CreateTicketCommentStoresAgentCommentKind|ListAssignedTicketsForUserAgentCompletionPending)'
```

**Step 6: Commit**

```bash
git add src/internal/common/control.go src/internal/common/oauth.go src/internal/common/control_integration_test.go
git commit -m "track agent completion comment state"
```

---

### Task 3: Add failing tests for web/API plumbing

**Files:**
- Modify: `src/cmd/web/api_v1.go`
- Modify: `src/cmd/web/routes_test.go`

**Step 1: Add a failing test for the new query parameter parsing**

Cover:

- `agent_completion_pending=true` is accepted
- invalid boolean returns `400`

**Step 2: Implement the API option plumbing**

Pass the new flag into `ListAssignedTicketsForUser`.

**Step 3: Re-run the targeted test**

```bash
env GOCACHE=/tmp/bv-gocache go test ./src/cmd/web -run 'Test.*Assigned.*AgentCompletionPending'
```

**Step 4: Commit**

```bash
git add src/cmd/web/api_v1.go src/cmd/web/routes_test.go
git commit -m "add agent completion pending filter"
```

---

### Task 4: Add failing tests for the agent run loop

**Files:**
- Modify: `src/cmd/agent/client.go`
- Modify: `src/cmd/agent/run.go`
- Modify: `src/cmd/agent/run_test.go`

**Step 1: Write failing tests**

Cover:

- agent fetches assigned tickets with the new pending-completion flag
- success path posts acknowledgement comment, progress updates, and final completion comment
- failure path posts acknowledgement comment and error updates, but no completion comment

**Step 2: Run the targeted tests and confirm failure**

```bash
env GOCACHE=/tmp/bv-gocache go test ./src/cmd/agent -run 'TestRunAgentOnce'
```

**Step 3: Implement the minimal agent changes**

Add:

- client support for `agent_completion_pending=true`
- helper to post tagged agent comments
- final completion comment on success
- updated success/failure expectations

**Step 4: Re-run the targeted tests**

```bash
env GOCACHE=/tmp/bv-gocache go test ./src/cmd/agent -run 'TestRunAgentOnce'
```

**Step 5: Commit**

```bash
git add src/cmd/agent/client.go src/cmd/agent/run.go src/cmd/agent/run_test.go
git commit -m "finish agent runs with completion comments"
```

---

### Task 5: Update e2e/spec/TLA docs and verify

**Files:**
- Modify: `src/cmd/agent/e2e_test.go`
- Modify: `doc/spec/agent.md`
- Modify: `doc/spec/common.md`
- Modify: `doc/spec/web.md`
- Modify: `doc/usage/agent-running-workflow.md`
- Modify: `doc/tla/AgentRun.tla`

**Step 1: Update e2e expectations**

Assert:

- two ticket comments exist: ack and completion
- updates still contain progress markers
- ticket remains pending until the completion comment is present

**Step 2: Update docs**

Reflect:

- new query parameter
- completion comment semantics
- removal of the known ack-first crash gap from the current implementation notes

**Step 3: Run package and full-suite verification**

```bash
env GOCACHE=/tmp/bv-gocache go test ./src/internal/common ./src/cmd/web ./src/cmd/agent
env GOCACHE=/tmp/bv-gocache go test ./...
```

**Step 4: Commit**

```bash
git add src/cmd/agent/e2e_test.go doc/spec/agent.md doc/spec/common.md doc/spec/web.md doc/usage/agent-running-workflow.md doc/tla/AgentRun.tla
git commit -m "document agent completion semantics"
```
