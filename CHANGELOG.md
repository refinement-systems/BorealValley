# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Security
- LOW: Template Execute error results silently discarded (#8)
- LOW: SQL queries using fmt.Sprintf for table names - fragile pattern (#7)
- MEDIUM: No login rate limiting - brute force and Argon2id DoS (#6)
- MEDIUM: requireAuth doesn't verify user still exists in DB (#5)
- HIGH: No request body size limit - DoS via unbounded JSON decode (#4)
- CRITICAL: IDOR on ticket tracker assignment - no CanAccessRepository check (#2)
- HIGH: Open redirect via backslash in return_to (FIXED) (#3)

### Added

### Fixed
- Fix ticket links to use relative paths and show summaries instead of UUIDs (#32)

### Changed
- Install Java runtime for TLA+ model checking (#43)
- spec does not reflect lessons from closed security and UX issues (#33)
- form textarea widths inconsistent with text inputs (#24)
- login page: card is full-width, no required field validation (#23)
- HTTP access shows bare Go error instead of redirecting to HTTPS (#15)
- assign tracker form stays visible after tracker already assigned (#22)
- priority field is an unlabeled number input with no guidance (#21)
- tracker detail page header redundantly shows name twice (#25)
- permission error for repo operations gives no explanation or path forward (#19)
- justfile has no target descriptions and integration tests excluded from 'just test' (#30)
- deprecated --keep-root flag still documented and wired up in justfile (#31)
- ctl validation errors use structured log format instead of plain error messages (#29)
- README.md is empty — no setup instructions for new developers (#28)
- CLAUDE.md references flags that no longer exist (-db, -addr) (#27)
- user profile page is an ActivityPub actor viewer, not a profile page (#26)
- repo list and detail expose container-internal filesystem paths (#17)
- ticket tracker detail: Create Ticket form silently disappears with no repos assigned (#18)
- admin dashboard exposes internal DB table names in Object Type Counts (#13)
- repo list and detail expose container-internal filesystem paths (#17)
- ticket detail page uses ActivityPub-internal terminology and UUIDs as titles (#20)
- admin dashboard shows no username — just 'Logged in.' (#16)
- error pages are bare unstyled plain text (#12)
- no persistent navigation — every page is an island (#14)
- UX and usability review (#9)
- Security review (#1)
