# Agent Running Workflow (Live Server)

This guide covers the practical workflow for running `BorealValley-agent` against a live BorealValley web server.

## 1. Standard Local Dev Restart

For the repository's standard local Docker setup, bring back the existing server and database with:

```bash
just dev-docker-up ~/repo/bvroot
```

That command reuses the existing root directory and Docker PostgreSQL volume, so it is the right command when you want the same users, tickets, OAuth apps, and prior agent runs to remain available in the UI.

In that standard setup:

- the host root path is `~/repo/bvroot`
- that same directory is mounted into the web container as `/work`
- PostgreSQL is reachable from the host at `127.0.0.1:5432`
- PostgreSQL is reachable from the web container at `db:5432`

`--root` must match the filesystem namespace of the process you are running:

- if you run `go run ./src/cmd/ctl ...` or `./tools/dev/agent/create-oauth-app.sh ...` on the host, use the host path such as `~/repo/bvroot`
- if you run `./tools/deploy/docker-dev-ctl.sh ...`, the command executes inside the running web container, so use `/work`

In the standard local setup, the web origin is:

```text
https://bv.local:4000
```

To stop without deleting that state:

```bash
just dev-docker-down ~/repo/bvroot
```

Do not use `just dev-docker-reset ~/repo/bvroot` when you intend to keep the existing database state. For the full local stack guide, see `docker-dev-stack.md`.

## 2. Prerequisites

- A running BorealValley web server (HTTPS recommended).
- PostgreSQL configured for the server.
- A local LM Studio instance with API enabled (default `http://127.0.0.1:1234`).
- A model loaded in LM Studio.
- Access to run `BorealValley-ctl` on the server side.
- Access to run `BorealValley-agent` on the machine where the agent will execute.

## 3. Server-Side Setup

For host-side commands in the examples below, define the standard local root once:

```bash
export ROOT="$HOME/repo/bvroot"
```

### 3.1 Recommended when the local Docker dev stack is already running

If you already started the standard local dev stack with `just dev-docker-up ~/repo/bvroot`, the simplest setup path is to run `BorealValley-ctl` inside the running web container.

In that case:

- use `--root /work`
- do not pass `--pg-dsn`
- do not export `BV_PG_DSN` on the host just for these `ctl` commands
- the container already has `BV_PG_DSN=postgres://app:app_pw@db:5432/app_db?sslmode=disable`

Create the agent user (or reuse an existing one):

```bash
./tools/deploy/docker-dev-ctl.sh adduser --root /work agentbot 'very-strong-password'
```

Create an OAuth app for the agent with required scopes:

```bash
./tools/deploy/docker-dev-ctl.sh oauth-app create \
  --root /work \
  --name "agentbot" \
  --description "BorealValley agent app" \
  --redirect-uri "http://127.0.0.1:8787/callback" \
  --scope profile:read \
  --scope repo:read \
  --scope ticket:read \
  --scope ticket:write
```

The command prints `client_id` and `client_secret`. Save both values securely.

### 3.2 Host-side variant

If you prefer to run `ctl` or helper scripts from the host shell while the Docker PostgreSQL container stays running, export the host-reachable DSN first:

```bash
export BV_PG_DSN='postgres://app:app_pw@127.0.0.1:5432/app_db?sslmode=disable'
```

Then use the host path for `--root`:

```bash
go run ./src/cmd/ctl adduser --root "$ROOT" --pg-dsn "$BV_PG_DSN" agentbot 'very-strong-password'
```

```bash
./tools/dev/agent/create-oauth-app.sh \
  --root "$ROOT" \
  --pg-dsn "$BV_PG_DSN" \
  --name "agentbot" \
  --redirect-uri "http://127.0.0.1:8787/callback"
```

Use the host DSN only for host-side commands. If you switch back to `./tools/deploy/docker-dev-ctl.sh`, switch `--root` back to `/work` and omit `--pg-dsn` again.

## 4. Create a Fresh Test Repository and Ticket

On a fresh setup, the agent can only process tickets that are:

- assigned to the agent user, and
- visible to that user via repository access.

### 4.1 Create and discover a local repository

Create a test repo under the host root path. In the standard Docker dev setup, that directory is mounted into the container as `/work/repo`:

```bash
mkdir -p "$ROOT/repo/demo"
pijul init "$ROOT/repo/demo"
```

Resync repositories into BorealValley:

```bash
go run ./src/cmd/ctl resync --root "$ROOT" --pg-dsn "$BV_PG_DSN"
```

Docker dev variant:

```bash
./tools/deploy/docker-dev-ctl.sh resync --root /work
```

### 4.2 Create tracker, assign repo, create ticket, assign agent

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

## 5. Agent Initialization (One-Time Per State File)

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

## 6. Run the Agent Once (Manual Testing)

```bash
./tools/dev/agent/run-once.sh \
  --workspace /path/to/workspace \
  --max-iter 3
```

Behavior:

1. Refreshes OAuth token if near expiry.
2. Fetches one assigned, completion-pending ticket.
3. Posts an acknowledgement root comment.
4. Fetches repository detail and resolves the agent-visible checkout source path from the repo API response.
5. Clones the repository into `--workspace/<repo-slug>/<ticket-slug>`.
6. Runs the LM Studio tool-calling loop inside that checkout.
7. Publishes progress/errors as `Update` entries attached to that acknowledgement comment.
8. If files changed, records one local Pijul change with message `<ticket-slug>: <summary>`.
9. Posts a separate completion root comment only after the run finishes successfully.

If no eligible ticket exists, the command exits successfully.

## 7. Verify Expected Results

After a run, open the processed ticket and verify:

- an acknowledgement comment was posted by the agent user,
- progress updates were appended to that acknowledgement comment during processing,
- on success, a second completion comment was posted by the agent user,
- on failure (for example low `--max-iter`), an `agent_error` update exists on the acknowledgement comment and no completion comment exists.

## 8. Useful Variants

Override model for a single run:

```bash
./tools/dev/agent/run-once.sh --workspace /path/to/workspace --model "another-model"
```

Use explicit state file:

```bash
./tools/dev/agent/run-once.sh --workspace /path/to/workspace --state-file /secure/path/agent-state.json
```

## 9. Troubleshooting

- `oauth state mismatch`: ensure browser redirected to the same `--redirect-uri` used during init.
- `http 401/403` on agent API calls: ensure OAuth app has scopes `profile:read repo:read ticket:read ticket:write`.
- Docker dev stopgap checkout support relies on translated repo paths from the server. It works for the standard `/work` to host-root mount and does not yet support remote/SSH repository checkout.
- local Pijul commit failures usually mean the agent machine does not have a usable Pijul identity configured yet. Run `pijul identity new borealvalley-agent` interactively once on the agent machine, then retry.
- `assignee has no repository access`: add `agentbot` as a member of the repository before assigning the ticket.
- `no assigned completion-pending tickets`: assign the ticket to the agent user and ensure that user has not already completed it with an agent completion comment.
- LM Studio errors: confirm API is enabled and the model name matches the loaded model.
