# Agent CLI (`BorealValley-agent`)

## 1. Entry Point

- binary: `src/cmd/agent/main.go`
- form: `agent COMMAND [OPTIONS]`
- commands:
  - `init`
  - `run`

## 2. State and Persistence

Auth/config state is persisted to XDG state storage:

- default path: `$XDG_STATE_HOME/BorealValley/agent/state.json`
- override path: `--state-file`
- file permissions: `0600`
- parent directory permissions: `0700`

Persisted fields include:

- server URL
- OAuth client id / client secret / redirect URI
- LM Studio model name and base URL
- OAuth token set (`access_token`, `refresh_token`, `expires_at`, etc.)
- current profile identity (`user_id`, `actor_id`, etc.)

Non-auth runtime execution state remains in-memory.

## 3. `agent init`

Command:

- `agent init --server-url --client-id --client-secret --redirect-uri --model [--lmstudio-url] [--state-file]`

Behavior:

1. Validates required flags.
2. Performs OAuth authorization-server discovery from:
   - `GET /.well-known/oauth-authorization-server`
3. Runs OAuth authorization-code flow with PKCE (`S256`) using scopes:
   - `profile:read ticket:read ticket:write`
4. Login UX:
   - loopback callback capture first
   - manual paste fallback if callback fails/times out
5. Exchanges auth code for token set.
6. Fetches profile (`GET /api/v1/profile`).
7. Writes state file in XDG state path.

## 4. `agent run`

Command:

- `agent run [--state-file] [--workspace] [--max-iter] [--model] [--lmstudio-url]`

Behavior per invocation:

1. Loads persisted state.
2. Applies optional runtime overrides (`--model`, `--lmstudio-url`).
3. Refreshes OAuth token when near expiry and persists rotated token/profile state.
4. Requests one assigned, completion-pending ticket:
   - `GET /api/v1/ticket/assigned?agent_completion_pending=true&limit=1`
5. If no eligible ticket exists, exits success.
6. If a ticket exists:
   - posts non-LLM acknowledgement root comment first
   - uses that created comment as the target for progress/error `Update` entries
   - posts a separate completion root comment only after successful finish
   - then runs one LM Studio tool-calling loop for this ticket

One invocation processes at most one ticket.

## 5. Ticket Envelope for Model Input

The agent sends a single user message envelope containing available ticket fields:

- ticket slug/id
- tracker slug
- repository slug
- `created_at`
- `priority`
- summary/title
- content/description

## 6. Tool Loop

LM Studio endpoint:

- `POST {lmstudio-url}/v1/chat/completions`

Available tools (sandboxed to `--workspace`):

- `list_dir`
- `read_file`
- `write_file`
- `search_text`

Default max iterations: `3` (`--max-iter` overrides).

If the loop hits iteration limit, `agent run` returns non-zero.

## 7. Publishing Progress and Errors

The agent publishes plain-text progress to updates attached to the acknowledgement comment:

- start marker
- tool call events
- tool results (truncated)
- assistant outputs (truncated)
- terminal error (including iteration-limit failures)

API used:

- `POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}/update`

Update text is appended with blank lines (`\n\n`) so the acknowledgement comment becomes a readable progress log.

## 8. Acknowledgement and Completion Comments

Before LLM execution, the agent posts an acknowledgement root comment via:

- `POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment`

The acknowledgement comment carries local metadata:

- `borealValleyAgentCommentKind = "ack"`

On successful completion, the agent posts one additional root comment:

- default content: `Agent completed ticket at <timestamp>.`
- if the assistant produced final text, it is appended after a blank line
- local metadata: `borealValleyAgentCommentKind = "completion"`

A ticket is considered complete for agent scheduling only after the completion comment exists. If the agent crashes after acknowledgement or progress updates but before completion, the ticket remains eligible for future runs.
