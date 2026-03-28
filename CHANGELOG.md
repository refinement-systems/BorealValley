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
- ticket detail page uses ActivityPub-internal terminology and UUIDs as titles (#20)
- admin dashboard shows no username — just 'Logged in.' (#16)
- error pages are bare unstyled plain text (#12)
- no persistent navigation — every page is an island (#14)
- UX and usability review (#9)
- Security review (#1)
