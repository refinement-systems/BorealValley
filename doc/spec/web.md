# Web Server

## 1. Binary and Runtime

Entry point: `src/cmd/web/main.go`

Command:

- `web serve --root <path> [--pg-dsn <dsn>] [--env dev|prod] [--cert FILE --key FILE] [--verbosity N]`

Runtime behavior:

- loads `$ROOT/config.json` via `--root`
- connects to PostgreSQL using `--pg-dsn` or `BV_PG_DSN`
- runs filesystem resync at startup
- initializes embedded OAuth server (Fosite)
- listens on `:<port>` from config

## 2. Session and CSRF Model

Session backend: `github.com/alexedwards/scs/v2`.

- lifetime: 24h
- idle timeout: 30m
- cookie path: `/`
- HttpOnly: true
- SameSite: Strict
- cookie name:
  - `session` in dev
  - `__Host-session` in prod

CSRF middleware (`OriginRefererCSRF`) applies to unsafe methods for cookie/session routes.
OAuth token/revocation endpoints and `/api/v1/*` bearer endpoints are exempt.

## 3. Routing

### 3.1 Public routes

- `GET /`
  - redirects to `/web/admin` when authenticated
  - redirects to `/web/login` otherwise
- `GET /web`
  - redirects to `/web/admin`
- `GET|POST /web/login`
  - supports optional `return_to` local path and redirects there after successful login
- `POST /web/logout`

### 3.2 OAuth Authorization Server routes

- `GET /.well-known/oauth-authorization-server`
- `GET /oauth/authorize`
- `POST /oauth/authorize`
- `POST /oauth/token`
- `POST /oauth/revoke`

Behavior summary:

- authorization code grant only (`response_type=code`)
- PKCE required (`S256` only)
- confidential clients only
- local user login required before consent
- consent grants can approve subset of requested scopes
- access token TTL: 1h
- refresh token TTL: 30d
- refresh token rotation on use

### 3.3 Session-auth web routes

All routes below require session `user_id`:

- `GET /users/{name}` (canonical local Person object URL)
- `GET /repo/{repo}` (canonical local Repository object URL)
- `GET /ticket-tracker/{tracker}` (canonical local TicketTracker object URL)
- `GET /ticket-tracker/{tracker}/ticket/{ticket}` (canonical local Ticket object URL)
- `GET /ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}` (canonical local Note comment object URL)
- `GET /web/admin`
- `GET /web/user/{name}`
- `GET /web/repo`
- `GET /web/repo/{repo}`
- `GET|POST /web/repo/{repo}/ticket-tracker`
- `POST /web/repo/{repo}/member` (admin-only add/remove repository members)
- `GET|POST /web/ticket-tracker`
- `GET /web/ticket-tracker/{tracker}`
- `POST /web/ticket-tracker/{tracker}/ticket`
- `POST /web/ticket-tracker/{tracker}/ticket/{ticket}/comment`
- `POST /web/ticket-tracker/{tracker}/ticket/{ticket}/assignee`
- `GET /web/ticket`
- `GET /web/notification`
- `POST /web/notification/clear`
- `POST /web/notification/reset`
- `POST /web/notification/{notification}`
- `GET /web/oauth/grant`
- `POST /web/oauth/grant/{grant}/revoke`

### 3.4 Bearer API routes (`/api/v1`)

All routes below require `Authorization: Bearer` with required scopes:

- `GET /api/v1/profile` (`profile:read`)
- `GET /api/v1/repo` (`repo:read`)
- `GET /api/v1/repo/{repo}` (`repo:read`)
- `GET /api/v1/ticket-tracker` (`tracker:read`)
- `POST /api/v1/ticket-tracker` (`tracker:write`)
- `GET /api/v1/ticket-tracker/{tracker}` (`tracker:read`)
- `POST /api/v1/repo/{repo}/ticket-tracker` (`repo:write` + `tracker:write`)
- `GET /api/v1/ticket-tracker/{tracker}/ticket` (`ticket:read`)
- `POST /api/v1/ticket-tracker/{tracker}/ticket` (`ticket:write`)
- `GET /api/v1/ticket/assigned` (`ticket:read`)
- `POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/update` (`ticket:write`)
- `GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/version` (`ticket:read`)
- `GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment` (`ticket:read`)
- `POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment` (`ticket:write`)
- `POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}/update` (`ticket:write`)
- `GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}/version` (`ticket:read`)
- `GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/assignee` (`ticket:read`)
- `POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/assignee` (`ticket:write`)
- `GET /api/v1/notification` (`notification:read`)
- `POST /api/v1/notification/clear` (`notification:write`)
- `POST /api/v1/notification/reset` (`notification:write`)
- `POST /api/v1/notification/{notification}` (`notification:write`)
- `GET /api/v1/repo/{repo}/member` (`repo:write`, admin-only)
- `POST /api/v1/repo/{repo}/member` (`repo:write`, admin-only)
- `GET /api/v1/object-count` (`repo:read`)
- `GET /api/v1/oauth/grant` (`profile:read`)
- `POST /api/v1/oauth/grant/{grant}/revoke` (`profile:read`)

### 3.5 Route naming convention

Object-kind route segments are singular, including list endpoints and multi-object paths.

### 3.6 Static assets

- `GET /static/js/*` serves embedded JS assets.

## 4. Removed Routes

Chat/model web routes were removed from the web binary:

- `/web/model`
- `/web/model/events/{id}`
- `/web/repo/{repo}/chat`
- `/web/repo/{repo}/chat/event/{job}`
- `/web/repo/{repo}/chat/reset`

Legacy non-web UI paths are intentionally not served (return `404`), including:

- `/admin...`
- `/login`
- `/logout`
- `/home`

## 5. Protocol Scope (Disabled)

ActivityPub gateway/protocol endpoints are intentionally disabled in this phase.

For local canonical object URLs (`/users/...`, `/repo/...`, `/ticket-tracker/...`),
the server supports authenticated dereference with content negotiation:

- `Accept: application/activity+json` or `application/ld+json` returns object JSON.
- other `Accept` values return HTML debug views of the same object URL.

ForgeFed federation/gateway behavior remains disabled; this is local object
dereference only.

## 6. Ticket and Comment Visibility

- Ticket and comment visibility is scoped by repository membership.
- Admin users (`users.is_admin = true`) bypass repository membership checks.
- Ticket list endpoints (`/web/ticket`, `/web/ticket-tracker/{tracker}`, and API ticket list routes) are filtered per authenticated user.
- Canonical ticket/comment object dereference requires access to the ticket's repository.
- Ticket comments are created as ActivityPub `Note` objects.
- Replies cannot widen visibility: reply recipient is always anchored to the root ticket `target`, and reply parent must belong to the same ticket and recipient scope.

## 7. Ticket Assignees and Notifications

- Ticket assignees can be listed and mutated for ticket routes.
- Assignment mutation accepts `action=add|remove` with `username`.
- Assignment is idempotent:
  - repeated `add` for same assignee is a no-op.
  - repeated `remove` for missing assignee is a no-op.
- Assigning a user generates an internal notification (`type: ticket_assigned`) only when assignment is newly added.
- Removing assignees does not create notifications.

Notification API behavior:

- `GET /api/v1/notification` returns a JSON array of items with:
  - `id`, `type`, `unread`, `created_at`, `account`, `ticket`.
- Query parameters:
  - `min_id` (optional, fetch newer)
  - `max_id` (optional, fetch older)
  - `limit` (optional, default `20`, max `100`)
- Pagination:
  - when another page is available, response includes `Link` header with `rel="next"`.
- Bulk state updates:
  - `POST /api/v1/notification/clear` sets all current-user notifications to `unread=false`.
  - `POST /api/v1/notification/reset` sets all current-user notifications to `unread=true`.
- Per-item state updates:
  - `POST /api/v1/notification/{notification}` with JSON body `{"unread": true|false}`.

Session web behavior mirrors API state controls through `/web/notification*` routes.

## 8. Ticket Priority and Update APIs

Ticket create payload (`POST /api/v1/ticket-tracker/{tracker}/ticket`) accepts:

- `repo` (required)
- `summary` (required)
- `content` (required)
- `priority` (optional integer; default `0`)

Assigned-ticket API (`GET /api/v1/ticket/assigned`):

- query `limit` (optional; default `20`, max `100`)
- query `unresponded` (optional boolean)
- query `agent_completion_pending` (optional boolean)
- returns tickets assigned to current user with:
  - tracker/ticket/repository slugs
  - `created_at`
  - `priority`
  - `responded_by_me`
- order is earliest-created first, then highest priority.
- `unresponded=true` excludes tickets where current user has any comment.
- `agent_completion_pending=true` excludes only tickets where current user has already posted a
  completion comment (`borealValleyAgentCommentKind = "completion"`).

Update APIs:

- ticket update: `POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/update`
- comment update: `POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}/update`
- body: `{"content":"..."}`; empty content is rejected (`400`).
- ACL follows existing repository/ticket visibility checks.

Ticket comment create payload (`POST /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment`) accepts:

- `content` (required)
- `in_reply_to` (optional)
- `agent_comment_kind` (optional; values `ack` or `completion`)

HTML canonical ticket and comment views render ticket/comment content with visible preserved newlines
using non-monospace `white-space: pre-wrap` containers.

Version APIs:

- ticket versions: `GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/version`
- comment versions: `GET /api/v1/ticket-tracker/{tracker}/ticket/{ticket}/comment/{comment}/version`
- query `limit` supported via common limit parser.
- response includes historical snapshot records from `ap_object_version`.
