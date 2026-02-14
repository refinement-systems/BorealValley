# Agent Running Workflow (Live Server)

This guide covers the practical workflow for running `BorealValley-agent` against a live BorealValley web server.

## 1. Prerequisites

- A running BorealValley web server (HTTPS recommended).
- PostgreSQL configured for the server.
- A local LM Studio instance with API enabled (default `http://127.0.0.1:1234`).
- A model loaded in LM Studio.
- Access to run `BorealValley-ctl` on the server side.
- Access to run `BorealValley-agent` on the machine where the agent will execute.

## 2. Server-Side Setup

Create the agent user (or reuse an existing one):

```bash
go run ./src/cmd/ctl adduser --root "$ROOT" --pg-dsn "$BV_PG_DSN" agentbot 'very-strong-password'
```

Create an OAuth app for the agent with required scopes:

```bash
./tools/dev/agent/create-oauth-app.sh \
  --root "$ROOT" \
  --pg-dsn "$BV_PG_DSN" \
  --name "agentbot" \
  --redirect-uri "http://127.0.0.1:8787/callback"
```

The script prints `client_id` and `client_secret`. Save both values securely.

## 3. Create a Fresh Test Repository and Ticket

On a fresh setup, the agent can only process tickets that are:

- assigned to the agent user, and
- visible to that user via repository access.

### 3.1 Create and discover a local repository

Create a test repo under `$ROOT/repo`:

```bash
mkdir -p "$ROOT/repo/demo"
git -C "$ROOT/repo/demo" init
```

Resync repositories into BorealValley:

```bash
go run ./src/cmd/ctl resync --root "$ROOT" --pg-dsn "$BV_PG_DSN"
```

Docker dev variant:

```bash
./tools/deploy/docker-dev-ctl.sh resync --root /work
```

### 3.2 Create tracker, assign repo, create ticket, assign agent

In the web UI (logged in as an admin or another user with repository access):

1. Open `/web/repo/demo` and add `agentbot` as a member in the **Members** section.
2. Open `/web/ticket-tracker` and create a tracker (for example `demo-tracker`).
3. Return to `/web/repo/demo`, assign the tracker in **Assign Tracker**.
4. Open `/web/ticket-tracker/<tracker-slug>`, create a ticket for repository `demo` (set optional `priority`).
5. Open the created ticket page and assign user `agentbot` in **Assign User**.

Agent processing order for assigned tickets is:

- earliest `created_at` first,
- then highest `priority`,
- then lowest internal ID.

The agent processes exactly one eligible ticket per `run` invocation.

## 4. Agent Initialization (One-Time Per State File)

Run init from the agent machine:

```bash
./tools/dev/agent/init-agent.sh \
  --server-url "https://bv.example.com" \
  --client-id "$CLIENT_ID" \
  --client-secret "$CLIENT_SECRET" \
  --model "<lmstudio-model-name>"
```

Defaults used by the script:

- redirect URI: `http://127.0.0.1:8787/callback`
- LM Studio URL: `http://127.0.0.1:1234`
- state file: XDG default (`$XDG_STATE_HOME/BorealValley/agent/state.json`)
- fresh login is forced by default; pass `--reuse-session` to reuse the current browser session

OAuth login flow:

- loopback callback capture first,
- manual paste fallback if callback is not possible.

## 5. Run the Agent Once (Manual Testing)

```bash
./tools/dev/agent/run-once.sh \
  --workspace /path/to/workspace \
  --max-iter 3
```

Behavior:

1. Refreshes OAuth token if near expiry.
2. Fetches one assigned, completion-pending ticket.
3. Posts an acknowledgement root comment.
4. Runs LM Studio tool-calling loop.
5. Publishes progress/errors as `Update` entries attached to that acknowledgement comment.
6. Posts a separate completion root comment only after the run finishes successfully.

If no eligible ticket exists, the command exits successfully.

## 6. Verify Expected Results

After a run, open the processed ticket and verify:

- an acknowledgement comment was posted by the agent user,
- progress updates were appended to that acknowledgement comment during processing,
- on success, a second completion comment was posted by the agent user,
- on failure (for example low `--max-iter`), an `agent_error` update exists on the acknowledgement comment and no completion comment exists.

## 7. Useful Variants

Override model for a single run:

```bash
./tools/dev/agent/run-once.sh --workspace /path/to/workspace --model "another-model"
```

Use explicit state file:

```bash
./tools/dev/agent/run-once.sh --workspace /path/to/workspace --state-file /secure/path/agent-state.json
```

## 8. Troubleshooting

- `oauth state mismatch`: ensure browser redirected to the same `--redirect-uri` used during init.
- `http 401/403` on agent API calls: ensure OAuth app has scopes `profile:read ticket:read ticket:write`.
- `assignee has no repository access`: add `agentbot` as a member of the repository before assigning the ticket.
- `no assigned completion-pending tickets`: assign the ticket to the agent user and ensure that user has not already completed it with an agent completion comment.
- LM Studio errors: confirm API is enabled and the model name matches the loaded model.
