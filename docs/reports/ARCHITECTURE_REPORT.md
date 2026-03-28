# Architectural Code Review — BorealValley

**Date:** 2026-03-29
**Scope:** Full codebase (`src/`, `tools/`, `doc/`)
**Method:** ADIHQ Framework (Analyze, Design, Evaluate, Handle, Quality)

---

## Executive Summary

BorealValley is a federated ticket-tracking platform built on ActivityPub/ForgeFed, with three Go binaries (`web`, `ctl`, `agent`) sharing a common package. The architecture is fundamentally sound: clean binary separation, proper use of `database/sql` with parameterized queries, modern cryptography (Argon2id, Ed25519, bcrypt), and layered HTTP middleware. The OAuth 2.0 implementation via Ory Fosite is industrial-grade with PKCE enforcement.

This review identifies **1 critical bug**, **5 warnings**, and **4 nitpicks** across nine evaluation dimensions.

---

## Findings

### 1. SQL Placeholder Mismatch — `UnassignTicketTrackerFromRepository`

* **Severity:** CRITICAL
* **Dimension:** Functional Correctness
* **Location:** `src/internal/common/control.go:853-858`
* **Violation:** The UPDATE query references `$2` but only one argument (`repo.InternalID`) is passed. This will cause a runtime error on every call — the unassign feature is broken.

```go
if _, err := tx.ExecContext(ctx,
    `UPDATE ff_repository
        SET ticket_tracker_internal_id = NULL,
            updated_at = now()
      WHERE internal_id = $2::uuid`,  // <-- $2, but only 1 arg provided
    repo.InternalID,
); err != nil {
```

* **Mandated Refactor:** Change `$2` to `$1`.

---

### 2. `Store` God Object

* **Severity:** WARNING
* **Dimension:** Structural Modularity
* **Location:** `src/internal/common/control.go` (~1650 lines), `src/internal/common/oauth.go` (~1200 lines)
* **Violation:** The `Store` struct accumulates ~60 public methods spanning user management, repository CRUD, ticket operations, notification handling, ActivityPub object storage, and the full OAuth 2.0 storage interface. This is a textbook God Object — a single type with too many reasons to change and too many dependencies. The file `control.go` alone is ~1650 lines; `oauth.go` adds another ~1200.

* **Mandated Refactor:** Extract domain-bounded sub-stores (e.g., `UserStore`, `TicketStore`, `OAuthStore`, `NotificationStore`) that share a `*sql.DB` but own their own query methods. The `Store` becomes a composition root. This reduces cognitive load and enables independent unit testing of each domain boundary.

---

### 3. N+1 Query Pattern in `ListNotificationsForUser`

* **Severity:** WARNING
* **Dimension:** Performance & Complexity
* **Location:** `src/internal/common/control.go:1089-1186`
* **Violation:** For each notification row returned from the database, `CanAccessRepository` is called individually, which itself runs up to two queries (one for `IsUserAdmin`, one for `IsRepositoryMember`). With a `queryLimit` of up to 1000 rows, this produces up to 2000 additional queries per request.

```go
for rows.Next() {
    // ...scan row...
    canAccess, err := s.CanAccessRepository(ctx, n.RepositorySlug, userID)
    // ...
}
```

* **Mandated Refactor:** Push the access check into the SQL query itself using a subquery or CTE that joins against `ff_repository_member` and the admin flag, eliminating the per-row round trip. Alternatively, batch-load accessible repository slugs before iterating.

---

### 4. In-Memory Rate Limiter Lacks Eviction

* **Severity:** WARNING
* **Dimension:** Performance & Complexity
* **Location:** `src/cmd/web/ratelimit.go:8-55`
* **Violation:** The `loginRateLimiter.attempts` map grows unboundedly. Expired entries are pruned only when the same key is accessed again. If an attacker sprays login attempts across many unique usernames, the map grows without bound, eventually exhausting memory.

* **Mandated Refactor:** Add a periodic background sweep (e.g., every `window` duration) to prune stale keys, or switch to a fixed-size LRU map with TTL eviction.

---

### 5. Agent `SearchText` Accepts Arbitrary Regex from Untrusted Input

* **Severity:** WARNING
* **Dimension:** Security and Access
* **Location:** `src/internal/common/agent/tools.go:96-134`
* **Violation:** `SearchText` compiles a user-supplied regex via `regexp.Compile(query)`. While Go's `regexp` is RE2-based (no catastrophic backtracking), a sufficiently complex pattern applied to every file via `filepath.WalkDir` across large directory trees can still cause significant CPU consumption. There is no timeout, no file-count limit, and no file-size limit on the walk.

* **Mandated Refactor:** Add a `context.Context` parameter with a deadline to bound execution time. Consider limiting the number of files walked (e.g., 10,000) and skipping files larger than a threshold (e.g., 1MB). These are practical defense-in-depth measures for an agent sandbox.

---

### 6. Dynamic SQL via `fmt.Sprintf` in `ListObjectTypeCounts`

* **Severity:** WARNING
* **Dimension:** Security and Access
* **Location:** `src/internal/common/control.go:1593-1610`
* **Violation:** Table names are interpolated into SQL via `fmt.Sprintf("SELECT COUNT(*) FROM %s", table)`. The `allowedTable()` guard provides adequate protection against injection — the values come from a hardcoded allowlist, not user input. However, this pattern is fragile: any future refactor that adds a table name from an untrusted source to the `objectTables` slice would silently introduce SQL injection.

* **Mandated Refactor:** Add a comment documenting the safety invariant (that `objectTables` is hardcoded and `allowedTable` is an allowlist). Alternatively, use `pq.QuoteIdentifier` or equivalent to quote the table name defensively.

---

### 7. `main.go` Contains CSRF Logic, Route Registration, TLS Setup, and Handler Initialization

* **Severity:** NITPICK
* **Dimension:** Cognitive Readability
* **Location:** `src/cmd/web/main.go` (551 lines)
* **Violation:** The web server's `main.go` owns the full CSRF middleware implementation (`OriginRefererCSRF`, `effectiveSchemeHost`, `parseForwardedFirst`, `normalizeHostPortForScheme`, `isClientIPTrusted`, `originOrRefererURL`), TLS listener setup, route registration, and the `main()` entrypoint. Meanwhile, `cross-site-request-forgery.go` exists but contains only a one-line comment redirecting to `main.go`. This makes `main.go` harder to navigate and violates SRP at the file level.

* **Mandated Refactor:** Move the CSRF-related functions into `cross-site-request-forgery.go` and delete the redirect comment. Optionally extract route registration into `routes.go`.

---

### 8. `requireAuth` Middleware Hits Database on Every Request

* **Severity:** NITPICK
* **Dimension:** Performance & Complexity
* **Location:** `src/cmd/web/login.go:104-119`
* **Violation:** `requireAuth` calls `app.store.UserExists()` (a database query) on every authenticated request to verify the user still exists. For a session-based application with a 30-minute idle timeout, this is overly defensive — users are very unlikely to be deleted mid-session. This adds one DB round trip per page load for every authenticated user.

* **Mandated Refactor:** This is acceptable for the current scale of the project. At higher traffic, consider caching the user-exists check (e.g., a short-lived in-memory cache or a session flag refreshed on login/renewal).

---

### 9. Inconsistent Error Response Format Between Web and API

* **Severity:** NITPICK
* **Dimension:** Architectural Alignment
* **Location:** `src/cmd/web/api_v1.go` (throughout), `src/cmd/web/data.go` (throughout)
* **Violation:** API endpoints return plain-text error bodies via `http.Error(w, "internal error", 500)` while web handlers return styled HTML error pages via `renderError()`. The API lacks a structured JSON error envelope (e.g., `{"error": "...", "code": "..."}`), which makes client-side error handling fragile — the agent must parse plain text to distinguish error types.

* **Mandated Refactor:** Introduce a `writeJSONError(w, statusCode, message)` helper and use it consistently in all `apiV1*` handlers. Include a machine-readable error code alongside the human-readable message.

---

### 10. `application` Struct Used as Implicit Dependency Container

* **Severity:** NITPICK
* **Dimension:** Testability Coverage
* **Location:** `src/cmd/web/main.go:46-53`
* **Violation:** The `application` struct holds concrete types (`*common.Store`, `*commonoauth.Runtime`, `*scs.SessionManager`). All handler methods are bound to `*application`, making them impossible to unit test without a fully initialized `Store` (which requires a PostgreSQL connection). No interfaces are used for dependency injection.

* **Mandated Refactor:** This is a known Go trade-off in small codebases. The current integration test approach (`integration_test.go`) is adequate. If the handler count continues to grow, consider extracting a `StoreReader` / `StoreWriter` interface boundary to enable handler-level unit tests with mocks.

---

## Positive Observations (Not Violations)

The following areas demonstrate strong engineering practices and should be preserved:

| Area | Assessment |
|------|-----------|
| **Password hashing** | Argon2id with proper salt, constant-time comparison, fake hash work for missing users (timing attack resistance). Best-in-class. |
| **CSRF protection** | Origin/Referer validation with trusted proxy CIDR allowlist. Correct handling of `Forwarded` and `X-Forwarded-*` headers. Port normalization for non-standard ports. |
| **OAuth 2.0** | PKCE mandatory, plain challenge method disabled. Proper token lifetimes (1h access, 30d refresh, 10m auth code). Redirect URI validation requires HTTPS except localhost-with-port. Client secrets use bcrypt. |
| **Session management** | Token renewal on login (session fixation prevention), `__Host-` prefix in prod, `SameSite=Strict`, `HttpOnly`, `Secure`. |
| **Template rendering** | Buffer-then-write strategy prevents partial HTML output on template errors. |
| **Agent sandbox** | Path traversal prevention via `filepath.Clean` + prefix check, rejecting absolute paths. 32KB read limit on files. |
| **SQL injection defense** | Parameterized queries used everywhere (except the guarded `ListObjectTypeCounts`). No string interpolation of user input into SQL. |
| **TLS configuration** | Minimum TLS 1.3 enforced. |
| **Formal verification** | TLA+ specification for agent run lifecycle (AgentRun.tla) with checked invariants. Rare and commendable. |
| **Request body limits** | 1MB max enforced at middleware level for all unsafe methods. |

---

## Review Matrix Summary

| # | Dimension | Findings |
|---|-----------|----------|
| 1 | Functional Correctness | 1 CRITICAL (SQL `$2`/`$1` mismatch) |
| 2 | Architectural Alignment | 1 NITPICK (inconsistent error formats) |
| 3 | Structural Modularity | 1 WARNING (`Store` god object) |
| 4 | Cognitive Readability | 1 NITPICK (`main.go` file scope) |
| 5 | Security and Access | 2 WARNING (unbounded regex walk, dynamic SQL comment) |
| 6 | Performance & Complexity | 2 WARNING (N+1 queries, rate limiter memory), 1 NITPICK (per-request user check) |
| 7 | Error Orchestration | — No violations found |
| 8 | Testability Coverage | 1 NITPICK (concrete deps in `application`) |
| 9 | Documentation & Intent | — No violations found |

**Total: 1 CRITICAL, 5 WARNING, 4 NITPICK**
