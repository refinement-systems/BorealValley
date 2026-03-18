# Repository Guidelines

## Database schema modifications
This is an experimental project that wasn't yet deployed anywhere.
No database migrations are necessary, implement all modifications to the database schema directly.

## Project Structure & Module Organization
Code lives under `src/` with command entrypoints in `src/cmd/`:
- `src/cmd/web/main.go`: primary web app binary (`serve`)
- `src/cmd/ctl/main.go`: secondary CLI entrypoint (`init-root`, `resync`, `adduser`, `oauth-app`)
- `src/internal/assets/`: embedded HTML templates (`html/` + `assets.go`)

Documentation is in `doc/`:
- `doc/spec/`: behavior and architecture notes
- `doc/external/`: external references

Project tooling and checks:
- `justfile`: build/clean targets
- `test/test.sh`: end-to-end shell smoke test (currently stale; see Testing)
- `bin/`: compiled outputs (`BorealValley-web`, `BorealValley-ctl`)

## Build, Test, and Development Commands
- `just build`: build both binaries into `bin/`.
- `just clean`: remove built binaries.
- `go test ./...`: run package tests (currently no `_test.go` files, but this is the baseline CI command).
- `just dev-docker-up ~/repo/bvroot`: start or resume the standard local Docker dev stack using the existing root directory and preserved PostgreSQL volume.
- `just dev-docker-down ~/repo/bvroot`: stop the standard local Docker dev stack without deleting state.
- `just dev-docker-reset ~/repo/bvroot`: recreate the standard local Docker dev stack with a fresh PostgreSQL volume; do not use this when you intend to keep yesterday's tickets, users, OAuth apps, or agent runs.
- `BV_PG_DSN='postgres://app:app_pw@127.0.0.1:5432/app_db?sslmode=disable' go run ./src/cmd/web serve --root ~/repo/bvroot --env dev`: run the web server locally against the standard development root and PostgreSQL instance.
- `go run ./src/cmd/ctl adduser --root ~/repo/bvroot --pg-dsn "$BV_PG_DSN" <user> <pass>`: seed a local user in PostgreSQL.
- `tools/deploy/docker-build.sh --platform linux/amd64 --tag borealvalley-web:latest`: build a production image without source code in the runtime layer.
- `tools/deploy/docker-buildx.sh --push --tag <registry>/<image>:<tag>`: build/publish cross-platform images with Docker Buildx (default platforms: `linux/amd64,linux/arm64`).

## Cross-Compile Compatibility
Cross-compiling is a project requirement.

- Keep `src/cmd/web` and `src/cmd/ctl` buildable with `CGO_ENABLED=0` for `linux/amd64` and `linux/arm64`.
- Avoid introducing cgo-only dependencies unless explicitly approved and documented.
- When changing build/runtime code, verify with:
  - `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./src/cmd/web ./src/cmd/ctl`
  - `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./src/cmd/web ./src/cmd/ctl`

## Coding Style & Naming Conventions
Use standard Go style and keep code `gofmt`-clean (tabs, idiomatic formatting). Follow existing naming patterns:
- Exported identifiers: `PascalCase` (`CSRFConfig`)
- Internal/private identifiers: `camelCase` (`trustedProxyCIDRs`)
- Constants/flags: concise, descriptive names (`dbPath`, `AllowInsecure`)

Prefer small functions, explicit error handling, and command-focused organization under `src/cmd/<name>`.

## Route Naming Convention
Route path segments representing object kinds must use singular names.
This applies even for collection/list endpoints and for routes spanning multiple objects.

## Testing Guidelines
Primary command: `go test ./...`.

For new behavior, add table-driven Go tests in `*_test.go` files next to the package under test. Keep names explicit (`TestLogin_InvalidCredentials`).

Note: `test/test.sh` currently expects `go build .` at repository root and fails in the current layout. Update it before relying on it in automation.

## Commit & Pull Request Guidelines
Recent commits use short, lowercase summaries (for example: `login works`, `dev mode works`). Keep commit messages brief and outcome-focused; prefer one logical change per commit.

For PRs, include:
- What changed and why
- How you validated it (`go test ./...`, manual web flow, etc.)
- Any spec/doc updates in `doc/spec/` when behavior changes
