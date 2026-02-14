# JSONB Performance and Duplication Audit (2026-02-28)

## Scope and Method

Static code audit of JSONB usage in:
- `src/internal/assets/sql/create.sql`
- `src/internal/common/control.go`
- `src/internal/common/oauth.go`
- `src/internal/common/control_activitypub.go`
- `src/internal/common/control_user_actor.go`

No live database profiling was performed (`BV_PG_DSN` unset).

Decision framework applied:
1. Reduce persisted duplication first.
2. Prefer simpler query plans when dedupe impact is equal.
3. Add indexes only for existing filter/sort shapes.
4. Avoid generated stored columns unless replacing duplication.
5. No GIN/JSONPath indexes without JSON predicate operators in filters.

---

## Key Findings

1. `jsonb` choice is correct across the schema; there are no `json` columns.
2. Active query predicates do **not** use JSON containment/existence operators (`@>`, `?`, `?|`, `?&`, `@?`, `@@`).
3. JSON scalar extraction (`->>`) appears only in `SELECT` projections in ticket list queries, not filters.
4. Most performance comes from relational keys and existing B-tree indexes, not JSONB indexing.
5. Highest-value issues are duplication drift risks between relational fields and mirrored JSON fields:
   - `ff_repository.ticket_tracker_internal_id` vs JSON links (`ticketsTrackedBy`/`tracksTicketsFor`)
   - `ff_ticket_assignee` vs `ff_ticket.body.assignedTo`
   - `ff_ticket_tracker.name/summary` vs `ff_ticket_tracker.body`
   - `ff_ticket.priority` vs `ff_ticket.body.priority`

---

## Query Shape Classification (control.go + oauth.go)

### Whole-document reads
- `SELECT ... body` and unmarshal in app code for object endpoints and comment/ticket rendering.
- Examples: `control.go:1697`, `1719`, `1743`, `1782`, `1838`; `oauth.go:1743`, `1782`.

### Scalar projection from JSON
- Ticket summary/content/published extracted via `->>` in `SELECT` list only.
- Examples: `oauth.go:1458-1460`, `1510-1512`, `1618-1619`.

### Scalar JSON filter
- None in active SQL (`WHERE`/`JOIN`/`ORDER BY`).

### JSON containment / key existence / JSONPath
- None in active SQL.

### Consequence
- Current code does not justify GIN (`jsonb_ops` or `jsonb_path_ops`) or JSON expression indexes.

---

## JSONB Usage Matrix (Every JSONB Column)

Legend:
- Query shape: `doc-read`, `scalar-proj`, `none`
- Duplication: `none`, `low`, `med`, `high`
- Index need now: `none`, `btree`, `expr`, `gin_ops`, `gin_path_ops`

| Table.Column | Write Path | Read Path | Filter/Sort on JSON | Duplication | Index Need Now | Notes |
|---|---|---|---|---|---|---|
| `as_person.body` | `control_activitypub.go:184-217`; `control_user_actor.go:40` | `control_user_actor.go:174-180` | none | low | none | Person payload stored/retrieved as document. |
| `as_application.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `as_ordered_collection.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `as_note.body` | `control_activitypub.go:184-217`; `control.go:1538-1543` | `control.go:1996`, `2044`; `oauth.go:1788`, `1850` | none | med | none | Metadata overlaps with `ff_ticket_comment` relational fields. |
| `as_update.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_team_membership.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_team.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_ticket_tracker.body` | `control.go:741-744`, `800-803`, `830-833`, `887-890`; `control_activitypub.go:184-217` | object reads in `control.go:1719`; joins via relational fields elsewhere | none | high | none | Duplicates `name/summary` columns and repository-link JSON mirror. |
| `ff_repository.body` | `control.go:1782-1793`, `819-825`, `877-883`; `control_activitypub.go:184-217` | object reads in `control.go:1697`; loaded in tx flows | none | high | none | Duplicates tracker linkage (`ticketsTrackedBy` vs FK). |
| `ff_patch_tracker.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_ticket.body` | `control.go:970-981`, `1076-1080`, `1121-1125`, `1473-1479`; `control_activitypub.go:184-217` | object reads in `control.go:1743`; projection in `oauth.go:1458-1460`, `1510-1512`, `1618-1619` | no JSON filter | high | none | Duplicates `priority` and assignee relationship mirror. |
| `ff_patch.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_commit.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_branch.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_factory.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_enum.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_enum_value.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_field.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_milestone.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_release.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_review.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_review_thread.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_code_quote.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_suggestion.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_approval.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_organization_membership.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_ssh_public_key.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_ticket_dependency.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_edit.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_grant.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_revoke.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_assign.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_apply.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_resolve.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ff_push.body` | `control_activitypub.go:184-217` | none (direct) | none | none | none | Generic object store. |
| `ap_object_unknown.body` | `control_activitypub.go:193-200` | none (direct) | none | none | none | Unknown AP types store.
| `ap_object_version.body` | `control.go:2105-2122` | `control.go:1632-1635` | none | high (intentional history) | none | Version snapshots are deliberate duplication for audit/history.
| `oauth_client.redirect_uris` | `oauth.go:231-233` | `oauth.go:373-374`, `435-436`, `718-723` | none | none | none | Stored JSON array; read by PK.
| `oauth_client.allowed_scopes` | `oauth.go:231-233` | `oauth.go:373-374`, `435-436`, `718-723` | none | none | none | Stored JSON array; read by PK.
| `oauth_consent_grant.requested_scopes` | `oauth.go:1237-1243` | `oauth.go:1266`, `1338` | none | low | none | Array snapshot per grant.
| `oauth_consent_grant.granted_scopes` | `oauth.go:1237-1244` | `oauth.go:1267`, `1339` | none | low | none | Array snapshot per grant.
| `oauth_authorization_code.request_json` | `oauth.go:829-847` | `oauth.go:858-863` | none | med | none | Duplicates some scalar columns by design.
| `oauth_pkce_request.request_json` | `oauth.go:914-920` | `oauth.go:927-930` | none | med | none | Snapshot store keyed by signature.
| `oauth_access_token.request_json` | `oauth.go:968-985` | `oauth.go:993-999` | none | med | none | Snapshot store keyed by signature.
| `oauth_refresh_token.request_json` | `oauth.go:1047-1070` | `oauth.go:1083-1088` | none | med | none | Snapshot store keyed by signature.

---

## Performance Fitness Assessment (per guidance)

### `jsonb` vs `json`
- Current usage is correct: all JSON columns are `JSONB`.
- No requirements found for preserving source key order/whitespace.

### GIN suitability
- Not currently needed.
- No workload evidence of JSON containment/existence predicates.

### Expression index suitability
- Not currently needed.
- JSON scalar extractions are projections, not filters.

### B-tree suitability
- Already dominant and correct for active query patterns via relational columns.

### TOAST / write amplification risks
- Highest risk areas are repeated full-document rewrites where relational mirrors are also maintained (`ff_repository.body`, `ff_ticket_tracker.body`, `ff_ticket.body`).

---

## Duplication Hotspots and Scores

Scale: 1 (low) to 5 (high)

| Hotspot | Duplicate Bytes | Drift Risk | Write Amplification | Read Complexity | Migration Risk | Rank |
|---|---:|---:|---:|---:|---:|---:|
| Tracker-repo linkage mirrored in JSON (`ticketsTrackedBy` + `tracksTicketsFor`) and FK (`ticket_tracker_internal_id`) | 3 | 5 | 5 | 4 | 3 | 1 |
| Ticket assignees mirrored in JSON (`assignedTo`) and table (`ff_ticket_assignee`) | 3 | 5 | 4 | 4 | 3 | 2 |
| Tracker `name/summary` mirrored in columns and JSON | 2 | 4 | 3 | 2 | 2 | 3 |
| Ticket `priority` mirrored in column and JSON | 1 | 3 | 3 | 2 | 2 | 4 |
| OAuth `request_json` plus scalar token/grant columns | 4 | 2 | 2 | 2 | 4 | 5 |
| Object version snapshots (`ap_object_version.body`) | 5 | 1 | 5 | 1 | 5 | 6 (intentional) |

---

## Ranked Recommendations

## Tier A (de-dupe now)

1. **Stop persisting tracker-repository relationship mirrors in local JSON bodies.**
- Remove write-time maintenance of:
  - `ff_repository.body.ticketsTrackedBy`
  - `ff_ticket_tracker.body.tracksTicketsFor`
- Keep relational FK `ff_repository.ticket_tracker_internal_id` as source of truth.
- Build/compose those AP fields at response/render time.
- Why now: highest drift + high write amplification, no JSON index benefit.
- Changes:
  - Schema: none required immediately.
  - Query/code: yes (object serialization path).
  - API payload: same fields preserved, but computed instead of stored.

2. **Stop persisting `ff_ticket.body.assignedTo`; derive from `ff_ticket_assignee`.**
- Keep `ff_ticket_assignee` as source of truth.
- Compose `assignedTo` in ticket JSON output when needed.
- Why now: high drift risk and avoid full ticket-body rewrite on assignment changes.
- Changes:
  - Schema: none required immediately.
  - Query/code: yes.
  - API payload: same field, computed.

3. **Remove `ff_ticket_tracker.name` and `ff_ticket_tracker.summary` columns (or stop using them).**
- Use JSON body as source or computed projection view.
- Why now: direct duplication with no filter/index dependency.
- Changes:
  - Schema: yes (drop columns) if full dedupe chosen.
  - Query/code: yes (`ListTicketTrackers`, `GetTicketTrackerBySlug`).
  - API payload: no shape change.

## Tier B (de-dupe later)

1. **Resolve ticket `priority` dual storage.**
- Prefer relational `ff_ticket.priority` as source of truth for sorting.
- Compute JSON `priority` in outward object serialization instead of storing duplicate in `body`.
- Changes:
  - Schema: optional (keep column; stop mirroring in body).
  - Query/code: yes.
  - API payload: no shape change.

2. **Rationalize OAuth `request_json` duplication only if token-table size becomes a proven issue.**
- Current duplication is functional for fosite snapshots; migration risk is high.
- Defer until operational evidence (table bloat/WAL pressure).
- Changes:
  - Schema/query/API: potentially significant, defer.

## Tier C (keep as-is)

1. **No JSON GIN indexes now.**
- No qualifying JSON predicate operators in live query shapes.

2. **No JSON expression indexes now.**
- Current JSON extraction is projection-only.

3. **Keep `ap_object_version.body` snapshot duplication.**
- Intentional history/audit data model.

4. **Keep JSONB-only generic object tables (`ff_*`, `as_*`, unknown types) unchanged until they gain filtered query workloads.**

---

## Validation Scenarios for Recommended Changes

1. Consistency drift checks
- Verify no divergence between relational source and emitted JSON for:
  - tracker assignment fields
  - assignees
  - priority

2. Query-shape/index fit checks
- Ensure any new index proposal maps to a real predicate/sort.
- Reject indexes for projection-only JSON access.

3. No-regression payload checks
- Compare object/API JSON before/after dedupe refactor (field-for-field parity).

4. Write amplification checks
- Measure update paths that currently rewrite full `body` for small relation changes:
  - tracker assignment
  - assignee add/remove

---

## Direct Answers to Requested Questions

1. **Is JSON/JSONB usage performant?**
- **Mostly yes for current workload.**
- JSONB type choice is correct.
- Current workload does not justify JSON-specific indexing.
- Main performance concern is duplicated-field write amplification, not missing JSON indexes.

2. **Is there duplicate data that could be reduced by better index/computed usage?**
- **Yes, substantial duplication exists in relational + JSON mirror fields.**
- Indexes do not solve this duplication.
- Best reduction path is to make relational data canonical for hot relations and compute mirrored JSON fields at read/serialization time (non-stored computation), starting with Tier A.
