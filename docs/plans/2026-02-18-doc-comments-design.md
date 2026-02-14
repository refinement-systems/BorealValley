# Design: Add Go doc comments to all functions

**Date:** 2026-02-18

## Goal

Add Go doc comments (per https://go.dev/doc/comment) to every function in the project, including unexported helpers. Use style Option B: a one-liner starting with the function name, plus brief notes on non-trivial behavior, error conditions, or security semantics where they add real value.

## Scope

All `.go` files under `src/`, excluding test files and files with no functions (`assets.go`).

## Per-file comment plan

### `src/internal/common/envVar.go`

- `RealPath` — Returns the absolute, symlink-resolved path. Fails if any component does not exist.
- `envOrDefault` — Returns the resolved directory for the given XDG env var, falling back to def if unset. Creates the directory if absent. Panics on filesystem errors or conflicting cached values.
- `EnvDirHome` — Returns the user's home directory from $HOME.
- `EnvDirData` — Returns the XDG data directory ($XDG_DATA_HOME/BorealValley, defaulting to ~/.local/share/BorealValley).
- `EnvDirConfig` — Returns the XDG config directory ($XDG_CONFIG_HOME/BorealValley).
- `EnvDirState` — Returns the XDG state directory ($XDG_STATE_HOME/BorealValley).

### `src/internal/common/logging.go`

- `SlogLevelFromVerbosity` — already has a doc comment; keep as-is.
- `ConfigureLogging` — Configures the default slog logger at the given verbosity level (0–4), writing to stderr.
- `newLogger` — Creates a text-format slog.Logger writing to out at the given level, with source location enabled.

### `src/internal/common/control.go`

- `ControlDbPathDefault` — Returns the default SQLite database path under the XDG data directory.
- `ControlPlaneInit` — Opens and initialises the SQLite control database at path, applying the schema. Returns an error if already initialised.
- `Close` — Closes the underlying SQLite connection and clears the singleton state.
- `CreateUser` — Creates a new user with the given username and argon2id-hashed password. Password must be at least 12 characters.
- `VerifyUser` — Verifies credentials against the database. Returns ok=true and the user ID on success. Performs dummy hash work for missing users to mitigate timing attacks.
- `fakeHashWork` — Performs a full argon2id hash to consume the same time as a real verification, preventing timing-based user enumeration.

### `src/cmd/web/main.go`

- `usage` — Prints command-line usage for the web server to stderr.
- `main` — Entry point: parses flags, initialises the control plane and session manager, wires the middleware chain, and starts the HTTP server.
- `home` — GET / — renders the home page.
- `putHandler` — POST /put — stores a demo message in the session, then redirects to /get.
- `getHandler` — GET /get — reads and returns the demo session message.
- `parseForwardedFirst` — Parses the first element of an RFC 7239 Forwarded header, returning the proto and host fields.
- `firstListItem` — Returns the first comma-separated value from a header field.
- `mustParseCIDRs` — Parses a slice of CIDR strings into net.IPNet values. Panics on any invalid CIDR.
- `hostEqual` — Reports whether two host strings are equal, case-insensitively.
- `sameOrigin` — Reports whether gotOrigin parses to the same scheme+host as expectedOrigin.
- `isUnsafeMethod` — Reports whether the HTTP method is unsafe (POST, PUT, PATCH, DELETE).
- `effectiveSchemeHost` — Returns the effective scheme and host for the request, consulting proxy headers if the request comes from a trusted proxy.
- `isHTTPS` — Reports whether the request was received over HTTPS, including via a trusted proxy.
- `isFromTrustedProxy` — Reports whether the request's remote address falls within one of the trusted CIDR ranges.

### `src/cmd/web/cross-site-request-forgery.go`

- `OriginRefererCSRF` — Middleware that validates the Origin or Referer header against the effective scheme+host for all unsafe HTTP methods, rejecting cross-origin requests.

### `src/cmd/web/login.go`

- `login` — GET renders the login form; POST authenticates credentials, renews the session token, and redirects to /.
- `logout` — POST destroys the session and redirects to /login.
- `requireAuth` — Middleware that redirects unauthenticated requests to /login.

### `src/cmd/ctl/main.go`

- `main` — Entry point: dispatches to the appropriate subcommand.
- `usage` — Prints top-level usage for the ctl binary to stderr.
- `addUserCmd` — Parses flags and positional arguments, then creates a new user via the control plane.
- `addUserUsage` — Prints usage for the adduser subcommand to stderr.
