# Control CLI (`BorealValley-ctl`)

## 1. Entry Point

- binary: `src/cmd/ctl/main.go`
- form: `ctl COMMAND [OPTIONS]`
- common options by command:
  - `--root` (default: `RootPathDefault()`)
  - `--pg-dsn` (or environment `BV_PG_DSN`, when command needs DB access)
  - `--verbosity` (`0..4`)

## 2. Commands

### 2.1. `init-root`

- initializes `$ROOT` if missing
- creates:
  - `$ROOT/config.json`
  - `$ROOT/repo/`

### 2.2. `resync`

- scans `$ROOT/repo` one level deep for repository directories
- only directories containing `.pijul` are treated as repositories
- upserts local ForgeFed `Repository` objects in PostgreSQL
- uses stable UUID marker file:
  - `.borealvalley-repo-id`

### 2.3. `adduser USER PASSWORD [--admin]`

- creates a local user in `users`
- validates:
  - username is non-empty after trim
  - password length is at least 12 characters
- provisions local ActivityPub actor identity for the user:
  - actor id: `{canonical_base_url}/users/{username}`
  - key id: `{actor_id}#main-key`
- sets `users.is_admin` when `--admin` is provided

### 2.4. `oauth-app ACTION`

OAuth third-party app registration is local-only through ctl.
No HTTP app registration endpoint exists in this phase.

Actions:

- `oauth-app create --name NAME --redirect-uri URI [--redirect-uri URI...] --scope SCOPE [--scope SCOPE...] [--description TEXT]`
  - creates a confidential OAuth client
  - validates redirect URI policy (HTTPS, or localhost HTTP with explicit port)
  - validates scopes against supported scope set
  - prints `client_id` and `client_secret` once
- `oauth-app rotate-secret --client-id ID`
  - rotates client secret immediately (no overlap)
  - prints new `client_secret` once
- `oauth-app enable --client-id ID`
  - enables client for new authorize/token requests
- `oauth-app disable --client-id ID`
  - disables client
  - revokes active grants, authorization codes, access tokens, and refresh tokens for that client
- `oauth-app list`
  - lists clients and enabled state
- `oauth-app show --client-id ID`
  - shows client metadata (no secret retrieval)

### 2.5. Agent OAuth Prerequisite

`BorealValley-agent init` expects a pre-registered confidential OAuth client.

Typical setup uses:

- `ctl oauth-app create ... --scope profile:read --scope repo:read --scope ticket:read --scope ticket:write`

The generated `client_id` and `client_secret` are then passed to `agent init`.

## 3. Error and Exit Behavior

- `-help`/`--help` on top-level or subcommands exits `0` after usage text
- invalid flags or invalid verbosity exit `2` with usage text
- runtime failures (root config, DB init, command operation failures) exit `1`

## 4. Implementation Notes

- CLI commands that access the store open and close a PostgreSQL-backed store once per invocation
- OAuth client secrets are hashed at rest and never returned from show/list
