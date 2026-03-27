# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
just build          # build both binaries into bin/
just test           # run package tests for both binaries
just build-web      # build bin/BorealValley-web only
just test-web       # go test ./src/cmd/web
just clean          # remove bin/* (keeps bin/.keep)

go test ./...       # run all tests (baseline CI command)
go test ./src/internal/common   # run a single package's tests

# Run the web server locally
go run ./src/cmd/web -db app.db -addr :4000 -env dev

# Add a user (min 12-char password)
go run ./src/cmd/ctl adduser -db app.db <username> <password>
```

## Architecture

Two binaries share the `src/internal/common` package:

- **`src/cmd/web`** — HTTP server. Flags: `-db`, `-addr`, `-env` (dev|prod), `-verbosity`.
- **`src/cmd/ctl`** — CLI admin tool. Subcommand: `adduser`.

### Database schema modifications
This is an experimental project that wasn't yet deployed anywhere.
No database migrations are necessary, implement all modifications to the database schema directly.

### Control plane (`src/internal/common/control.go`)

`ControlPlane` is a singleton wrapping a single-connection SQLite DB (via `modernc.org/sqlite`). It owns user management: `CreateUser` (argon2id hashing) and `VerifyUser` (constant-time comparison with timing-attack mitigation for missing users). The DB schema is embedded at `src/internal/assets/sql/create.sql` and applied on every `ControlPlaneInit`.

### Web layer (`src/cmd/web/`)

- **Session management**: `github.com/alexedwards/scs/v2` with a 24h lifetime / 30m idle timeout. Dev mode uses a plain `session` cookie; prod uses `__Host-session` with `Secure: true`.
- **CSRF protection**: `OriginRefererCSRF` middleware (`cross-site-request-forgery.go`) — checks `Origin`/`Referer` headers against the effective scheme+host for all unsafe methods (POST/PUT/PATCH/DELETE). Proxy headers (`Forwarded`, `X-Forwarded-Proto`, `X-Forwarded-Host`) are trusted only from the configured CIDRs (default: `127.0.0.1/32`, `::1/128`).
- **Middleware chain** (outermost first): `OriginRefererCSRF` → `sm.LoadAndSave` → mux handlers.
- **Templates**: HTML templates are embedded via `//go:embed` in `src/internal/assets/assets.go` and parsed at startup with `template.Must`.

### Directory conventions

Paths follow XDG base directory spec (see `doc/external/XDG-base-directory.md`). `EnvDirData()` returns `$XDG_DATA_HOME/BorealValley` (default `~/.local/share/BorealValley`), which is where the default DB lives.

## Commit style

Short, lowercase, outcome-focused: e.g. `login works`, `better path handling`. One logical change per commit.

## Testing

Use ~/repo/bvroot for --root of the main server.
Point agents to use the .temp/work directory for their scratchpads. Create if it neccessary.

