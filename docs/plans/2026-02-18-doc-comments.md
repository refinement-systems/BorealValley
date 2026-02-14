# Doc Comments Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Go doc comments (Option B style: function-name-first one-liner plus brief notes on non-trivial behavior) to every function in the project.

**Architecture:** Pure documentation pass — no logic changes. Each task touches one file, verifies the build still compiles, and commits. No test changes needed; `go build ./...` is the verification step.

**Tech Stack:** Go, standard `go build` / `go vet` tooling.

**Reference:** https://go.dev/doc/comment — doc comments must immediately precede the declaration with no blank line between them.

---

### Task 1: `src/internal/common/envVar.go`

**Files:**
- Modify: `src/internal/common/envVar.go`

**Step 1: Add doc comments**

Open `src/internal/common/envVar.go`. Add the following comments immediately above each function (no blank line between comment and `func`):

```go
// RealPath returns the absolute, symlink-resolved path of the given path.
// It fails if any component of the path does not exist.
func RealPath(path string) (string, error) {
```

```go
// envOrDefault returns the resolved directory path for the given XDG environment
// variable, falling back to def when the variable is unset. If addAppDir is true,
// "BorealValley" is appended to the path. The directory is created (mode 0700) if it
// does not exist. Results are cached; the function panics on filesystem errors or if a
// conflicting value is ever produced.
func envOrDefault(varname string, cache *envVal, def string, addAppDir bool) string {
```

```go
// EnvDirHome returns the user's home directory from $HOME, creating it if absent.
func EnvDirHome() string {
```

```go
// EnvDirData returns the XDG data directory for BorealValley
// ($XDG_DATA_HOME/BorealValley, defaulting to ~/.local/share/BorealValley).
func EnvDirData() string {
```

```go
// EnvDirConfig returns the XDG configuration directory for BorealValley
// ($XDG_CONFIG_HOME/BorealValley, defaulting to ~/.config/BorealValley).
func EnvDirConfig() string {
```

```go
// EnvDirState returns the XDG state directory for BorealValley
// ($XDG_STATE_HOME/BorealValley, defaulting to ~/.local/state/BorealValley).
func EnvDirState() string {
```

**Step 2: Verify**

```
go build ./src/internal/common/...
go vet ./src/internal/common/...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add src/internal/common/envVar.go
git commit -m "doc comments for envVar"
```

---

### Task 2: `src/internal/common/logging.go`

**Files:**
- Modify: `src/internal/common/logging.go`

**Step 1: Add doc comments**

`SlogLevelFromVerbosity` already has a doc comment — leave it unchanged.

Add above `ConfigureLogging`:

```go
// ConfigureLogging installs a text-format slog logger on the default logger at the
// level corresponding to verbosity (0–4). Output goes to stderr.
func ConfigureLogging(verbosity int) error {
```

Add above `newLogger`:

```go
// newLogger returns a new slog.Logger that writes text-format records to out at the
// given level, with source file and line number included in each record.
func newLogger(level slog.Level, out io.Writer) *slog.Logger {
```

**Step 2: Verify**

```
go build ./src/internal/common/...
go vet ./src/internal/common/...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add src/internal/common/logging.go
git commit -m "doc comments for logging"
```

---

### Task 3: `src/internal/common/control.go`

**Files:**
- Modify: `src/internal/common/control.go`

**Step 1: Add doc comments**

Add above `ControlDbPathDefault`:

```go
// ControlDbPathDefault returns the default path for the control plane SQLite database,
// located under the XDG data directory.
func ControlDbPathDefault() string {
```

Add above `ControlPlaneInit`:

```go
// ControlPlaneInit opens the SQLite control database at path, applies the schema, and
// returns the singleton ControlPlane. It returns ControlPlaneAlreadyInitialized if
// called more than once.
func ControlPlaneInit(path string) (*ControlPlane, error) {
```

Add above `Close`:

```go// Close closes the underlying SQLite connection and resets the singleton so that
// ControlPlaneInit may be called again.
func (self *ControlPlane) Close() {
```

Add above `CreateUser`:

```go
// CreateUser creates a new user record with the given username and an argon2id hash of
// password. The password must be at least 12 characters; the username is trimmed of
// leading and trailing whitespace and must not be empty.
func (self *ControlPlane) CreateUser(username, password string) error {
```

Add above `VerifyUser`:

```go
// VerifyUser looks up username in the database and checks the supplied password against
// the stored argon2id hash. On success it returns the user's row ID and ok=true. It
// returns ok=false with a nil error for invalid credentials. To mitigate timing-based
// user enumeration, it performs a full argon2id hash even when the user does not exist.
func (self *ControlPlane) VerifyUser(ctx context.Context, username, password string) (userID int64, ok bool, err error) {
```

Add above `fakeHashWork`:

```go
// fakeHashWork runs a full argon2id derivation with a random salt so that the time
// spent on a missing-user lookup matches a real password verification, preventing
// timing-based user enumeration.
func fakeHashWork(password string) {
```

**Step 2: Verify**

```
go build ./src/internal/common/...
go vet ./src/internal/common/...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add src/internal/common/control.go
git commit -m "doc comments for control plane"
```

---

### Task 4: `src/cmd/web/cross-site-request-forgery.go`

**Files:**
- Modify: `src/cmd/web/cross-site-request-forgery.go`

**Step 1: Add doc comment**

Replace the existing section comment `// ---------------- CSRF middleware ----------------` and add a proper doc comment immediately above `OriginRefererCSRF`:

```go
// OriginRefererCSRF returns an http.Handler that enforces same-origin CSRF protection
// for all unsafe HTTP methods (POST, PUT, PATCH, DELETE). It checks the Origin header
// first; if absent it falls back to the Referer header. Both are validated against the
// effective scheme and host derived from the request (consulting trusted-proxy headers
// when applicable). Requests that fail the check are rejected with 400 or 403.
func OriginRefererCSRF(cfg CSRFConfig, next http.Handler) http.Handler {
```

Note: remove the `// ---------------- CSRF middleware ----------------` section comment above it — the doc comment replaces it.

**Step 2: Verify**

```
go build ./src/cmd/web/...
go vet ./src/cmd/web/...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add src/cmd/web/cross-site-request-forgery.go
git commit -m "doc comment for OriginRefererCSRF"
```

---

### Task 5: `src/cmd/web/login.go`

**Files:**
- Modify: `src/cmd/web/login.go`

**Step 1: Add doc comments**

Add above `login`:

```go
// login handles the login form. GET renders the form; POST reads the username and
// password fields, verifies credentials via the control plane, renews the session
// token on success, and redirects to /. Invalid credentials re-render the form with
// an error message.
func (app *application) login(w http.ResponseWriter, r *http.Request) {
```

Add above `logout`:

```go
// logout destroys the current session and redirects the user to /login.
// Only POST is accepted.
func (app *application) logout(w http.ResponseWriter, r *http.Request) {
```

Add above `requireAuth`:

```go
// requireAuth is a middleware that redirects unauthenticated requests to /login.
// A request is considered authenticated when "user_id" is present in the session.
func (app *application) requireAuth(next http.HandlerFunc) http.HandlerFunc {
```

**Step 2: Verify**

```
go build ./src/cmd/web/...
go vet ./src/cmd/web/...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add src/cmd/web/login.go
git commit -m "doc comments for login handlers"
```

---

### Task 6: `src/cmd/web/main.go` — helper functions

**Files:**
- Modify: `src/cmd/web/main.go`

**Step 1: Add doc comments to helper functions**

Remove the section comment `// ---------------- Helper functions ----------------` and replace with doc comments on each function.

Add above `parseForwardedFirst`:

```go
// parseForwardedFirst parses the first list element of an RFC 7239 Forwarded header
// value and returns the proto and host parameter values. ok is true if either proto or
// host was present.
func parseForwardedFirst(v string) (proto, host string, ok bool) {
```

Add above `firstListItem`:

```go
// firstListItem returns the first comma-separated item from a header field value,
// trimming surrounding whitespace. It returns an empty string for an empty input.
func firstListItem(v string) string {
```

Add above `mustParseCIDRs`:

```go
// mustParseCIDRs parses each CIDR string in cidrs into a net.IPNet.
// It panics if any string is not a valid CIDR notation.
func mustParseCIDRs(cidrs []string) []*net.IPNet {
```

Add above `hostEqual`:

```go
// hostEqual reports whether host strings a and b are equal, ignoring ASCII case.
func hostEqual(a, b string) bool {
```

Add above `sameOrigin`:

```go
// sameOrigin reports whether gotOrigin (an Origin header value) resolves to the same
// scheme and host as expectedOrigin. Returns false for malformed or opaque origins.
func sameOrigin(gotOrigin, expectedOrigin string) bool {
```

Add above `isUnsafeMethod`:

```go
// isUnsafeMethod reports whether m is an HTTP method that modifies state
// (POST, PUT, PATCH, or DELETE).
func isUnsafeMethod(m string) bool {
```

Add above `effectiveSchemeHost`:

```go
// effectiveSchemeHost returns the scheme and host for the request as seen by the
// client. When the request arrives from a trusted proxy it reads the Forwarded or
// X-Forwarded-Proto / X-Forwarded-Host headers; otherwise it uses r.TLS and r.Host
// directly. ok is false when no host can be determined.
func effectiveSchemeHost(r *http.Request, trusted []*net.IPNet) (scheme, host string, ok bool) {
```

Add above `isHTTPS`:

```go
// isHTTPS reports whether the request was made over HTTPS, either directly (r.TLS !=
// nil) or as signalled by a trusted proxy via X-Forwarded-Proto or Forwarded.
func isHTTPS(r *http.Request, trusted []*net.IPNet) bool {
```

Add above `isFromTrustedProxy`:

```go
// isFromTrustedProxy reports whether the remote address of the request falls within
// one of the trusted CIDR ranges.
func isFromTrustedProxy(remoteAddr string, trusted []*net.IPNet) bool {
```

**Step 2: Verify**

```
go build ./src/cmd/web/...
go vet ./src/cmd/web/...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add src/cmd/web/main.go
git commit -m "doc comments for web helper functions"
```

---

### Task 7: `src/cmd/web/main.go` — top-level functions

**Files:**
- Modify: `src/cmd/web/main.go`

**Step 1: Add doc comments to top-level and handler functions**

Remove the section comment `// ---------------- HTTP handlers ----------------` and replace with doc comments on each handler.

Add above `usage`:

```go
// usage prints command-line usage information for the web server binary to stderr.
func usage() {
```

Add above `main`:

```go
// main parses command-line flags, initialises the control plane and session manager,
// configures the CSRF and session middleware chain, and starts the HTTP server.
func main() {
```

Add above `home`:

```go
// home handles GET / and renders the home page template.
func (app *application) home(w http.ResponseWriter, r *http.Request) {
```

Add above `putHandler`:

```go
// putHandler handles POST /put, storing a demo message in the session and redirecting
// to /get.
func (app *application) putHandler(w http.ResponseWriter, r *http.Request) {
```

Add above `getHandler`:

```go
// getHandler handles GET /get and writes the demo session message to the response.
func (app *application) getHandler(w http.ResponseWriter, r *http.Request) {
```

**Step 2: Verify**

```
go build ./src/cmd/web/...
go vet ./src/cmd/web/...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add src/cmd/web/main.go
git commit -m "doc comments for web entry point and HTTP handlers"
```

---

### Task 8: `src/cmd/ctl/main.go`

**Files:**
- Modify: `src/cmd/ctl/main.go`

**Step 1: Add doc comments**

Add above `main`:

```go
// main is the entry point for the ctl admin binary. It dispatches to the appropriate
// subcommand based on os.Args[1].
func main() {
```

Add above `usage`:

```go
// usage prints top-level usage information for the ctl binary to stderr.
func usage() {
```

Add above `addUserCmd`:

```go
// addUserCmd implements the "adduser" subcommand. It parses flags and the positional
// USER and PASSWORD arguments, initialises the control plane, and creates the user.
func addUserCmd(args []string) {
```

Add above `addUserUsage`:

```go
// addUserUsage prints usage information for the "adduser" subcommand to stderr.
func addUserUsage() {
```

**Step 2: Verify**

```
go build ./src/cmd/ctl/...
go vet ./src/cmd/ctl/...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add src/cmd/ctl/main.go
git commit -m "doc comments for ctl binary"
```

---

### Task 9: Final verification

**Step 1: Run full build and tests**

```
go build ./...
go vet ./...
go test ./...
```

Expected: all pass, no errors.

**Step 2: Check doc output looks correct**

```
go doc ./src/internal/common/
go doc ./src/cmd/web/
go doc ./src/cmd/ctl/
```

Skim the output to confirm comments render as expected.
