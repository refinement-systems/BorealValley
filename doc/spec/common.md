# Common Functionality

## 1. Scope

This specification covers shared behavior implemented in:

- `src/internal/common/`
- `src/internal/common/oauth/`
- `src/internal/common/agent/`
- `src/internal/assets/sql/create.sql`

## 2. Root Directory and Config

BorealValley is configured by a single root directory `$ROOT`.

`$ROOT` contains:

- `config.json`
  - `hostname` (string; either `host[:port]` or `http(s)://host[:port]`)
  - `port` (integer)
- `repo/`

`ctl init-root --root <path>` creates:

- `$ROOT/config.json` with default values (`hostname: bv.local`, `port: 4000`)
- `$ROOT/repo/`

## 3. Logging

Verbosity mapping:

- `4`: debug
- `3`: info
- `2`: warn
- `1`: error
- `0`: silent

Values outside `0..4` are invalid.

## 4. Database

Storage backend is PostgreSQL only.

Connection source:

- CLI flag `--pg-dsn`
- otherwise env var `BV_PG_DSN`

Schema is applied directly from embedded SQL on startup.
No migrations are used.

## 5. Authentication and Users

User records are stored in `users` with Argon2id password hashes.

`CreateUser(username, password)`:

- trims username
- rejects empty username
- enforces password length `>= 12`
- hashes with Argon2id and per-user random salt
- provisions a local actor identity

`VerifyUser(username, password)`:

- returns `(userID, true)` on success
- returns `(0, false, nil)` for invalid credentials
- performs fake hash work on unknown users to reduce timing leakage

## 6. OAuth Embedded Authorization Server

OAuth server is embedded via Ory Fosite.

Supported flows:

- `authorization_code`
- `refresh_token`

Not supported in this phase:

- `client_credentials`
- OIDC
- introspection endpoint exposure (internal introspection is used by bearer middleware)

Scope model:

- `profile:read`
- `repo:read`
- `repo:write`
- `tracker:read`
- `tracker:write`
- `ticket:read`
- `ticket:write`
- `notification:read`
- `notification:write`

Security rules:

- PKCE required (`S256`)
- confidential clients only
- redirect URI exact match against registered list
- redirect URI policy: HTTPS, or localhost HTTP with explicit port
- client secret hashed at rest
- bearer tokens stored by signature only (opaque token handling)
- client disable revokes active grants/tokens for that client

Token lifetimes:

- access token: 1 hour
- refresh token: 30 days

Persistence tables:

- `oauth_client`
- `oauth_client_secret`
- `oauth_consent_grant`
- `oauth_authorization_code`
- `oauth_pkce_request`
- `oauth_access_token`
- `oauth_refresh_token`
- `oauth_client_assertion_jti`

## 7. Filesystem Repository Discovery

`$ROOT/repo` is source of truth for local repositories.

Discovery rules:

- one level of repository directories under `$ROOT/repo`
- a repository is recognized only if `.pijul` exists in that directory

On `ResyncFromFilesystem`:

- local `ff_repository` rows are marked non-local
- discovered repositories are upserted as local objects
- each discovered repository gets a stable UUID marker file:
  - `.borealvalley-repo-id`

## 8. TicketTracker, Ticket, Comment, and Update Storage

Ticket tracking is database-only and independent from filesystem/worktrees.

- repositories may have nullable `ticket_tracker_internal_id`
- ticket trackers are local DB objects with actor IDs `/ticket-tracker/{tracker}`
- tickets are local DB objects under tracker namespace:
  - `/ticket-tracker/{tracker}/ticket/{ticket}`
  - ticket rows include numeric `priority` (`INTEGER`, default `0`)
- ticket comments are local ActivityPub `Note` objects:
  - `/ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}`
  - note JSON payload in `as_note`
  - conversation metadata in `ff_ticket_comment`
  - agent-created root comments may include local note metadata field `borealValleyAgentCommentKind`
    with values `ack` or `completion`
- ticket/comment update events are local ActivityPub `Update` objects:
  - update payload in `as_update`
  - update API writes also snapshot pre-update object body to `ap_object_version`
- local object IDs are dereferenceable by authenticated clients in the web
  binary, returning either ActivityPub JSON (`application/activity+json` or
  `application/ld+json`) or HTML based on `Accept`

Assignment updates repository and tracker linkage atomically.

Repository membership and visibility rules:

- Membership table: `ff_repository_member(repository_internal_id, user_id)`.
- Ticket creator is auto-added as a repository member when creating a ticket.
- Read/write access to tickets and comments is controlled by repository membership.
- Admin users (`users.is_admin`) bypass repository membership checks.
- Reply scope follows root ticket recipient (`target`) and cannot widen:
  - parent reply target must be on the same ticket
  - parent and reply recipient actor IDs must match

Ticket assignment and notification rules:

- Ticket assignee table: `ff_ticket_assignee(ticket_internal_id, user_id, assigned_by_user_id)`.
- Notifications table: `notification(user_id, kind, ticket_internal_id, repository_internal_id, assigned_by_user_id, unread)`.
- Supported notification kind in this phase: `ticket_assigned`.
- Assignment updates:
  - acting user must have repository access.
  - assignee must already have repository access.
  - `add` is idempotent and creates one unread notification only when assignment is newly created.
  - `remove` is idempotent and does not create notifications.
  - ticket object body keeps `assignedTo` synchronized with assignee actor IDs.
- Notification visibility is ACL-filtered by current repository access (same access model as tickets/comments).
- Unread state is explicit (`unread BOOLEAN`) and can be updated per-item or in bulk.

Update and version semantics:

- `CreateTicketUpdate` / `CreateTicketCommentUpdate` append plain text to target object `content`.
- `source.content` is kept in sync with appended content.
- each update snapshots the object body before mutation into `ap_object_version`.
- version list endpoints return snapshot history ordered newest-first.

Assigned ticket query semantics:

- `ListAssignedTicketsForUser` supports `Limit`, `UnrespondedOnly`, and `AgentCompletionPendingOnly`.
- ordering is deterministic: `created_at ASC`, then `priority DESC`, then `id ASC`.
- response includes `responded_by_me` (true when current user has any comment on ticket).
- `UnrespondedOnly` keeps human semantics: exclude tickets where current user has any comment.
- `AgentCompletionPendingOnly` excludes only tickets where current user already has a comment with
  `borealValleyAgentCommentKind = "completion"`.
- Agent acknowledgement comments or ordinary comments do not remove a ticket from the
  completion-pending set.

## 9. Agent Shared Runtime Helpers

Reusable file/tool helpers for the agent loop are in `src/internal/common/agent`.
