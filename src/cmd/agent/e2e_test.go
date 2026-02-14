// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED “AS IS” AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

//go:build e2e

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

const (
	e2eAdminPassword = "admin-password-123"
	e2eAgentPassword = "agent-password-123"
)

func TestE2EAgentOAuthToRunFlow(t *testing.T) {
	adminDSN := skipUnlessE2E(t)
	pgDSN := createTempDatabase(t, adminDSN)

	root := filepath.Join(t.TempDir(), "root")
	repoRoot := repoRootDir(t)
	if err := common.InitRoot(root); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	httpsPort := reserveFreePort(t)
	writeRootConfig(t, root, httpsPort)

	repoSlug, repoPath := createDummyRepo(t, root, "e2e-agent-repo")
	bins := buildE2EBinaries(t, repoRoot)

	baseURL := fmt.Sprintf("https://bv.local:%d", httpsPort)
	webProc := startProcess(t, repoRoot, bins.Web,
		"serve",
		"--root", root,
		"--pg-dsn", pgDSN,
		"--env", "prod",
		"--cert", filepath.Join(repoRoot, "cert", "bv.local+3.pem"),
		"--key", filepath.Join(repoRoot, "cert", "bv.local+3-key.pem"),
		"--verbosity", "4",
	)
	waitForHTTPSReady(t, baseURL+"/web/login", webProc)

	runCommand(t, "", bins.Ctl,
		"adduser",
		"--root", root,
		"--pg-dsn", pgDSN,
		"--admin",
		"--verbosity", "0",
		"admin",
		e2eAdminPassword,
	)
	runCommand(t, "", bins.Ctl,
		"adduser",
		"--root", root,
		"--pg-dsn", pgDSN,
		"--verbosity", "0",
		"agentbot",
		e2eAgentPassword,
	)

	redirectPort := reserveFreePort(t)
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", redirectPort)
	oauthOut := runCommand(t, "", bins.Ctl,
		"oauth-app",
		"create",
		"--root", root,
		"--pg-dsn", pgDSN,
		"--verbosity", "0",
		"--name", "agentbot-e2e",
		"--redirect-uri", redirectURI,
		"--scope", "profile:read",
		"--scope", "ticket:read",
		"--scope", "ticket:write",
	)
	clientID := parseKeyValueOutput(t, oauthOut, "client_id")
	clientSecret := parseKeyValueOutput(t, oauthOut, "client_secret")

	webClient := newE2EWebClient(t)
	loginWebUser(t, webClient, baseURL, "admin", e2eAdminPassword)
	trackerSlug, ticketSlug := createAdminTaskFlow(t, webClient, baseURL, repoSlug, "agentbot")

	statePath := filepath.Join(t.TempDir(), "agent-state.json")
	initProc, authURLCh := startAgentInit(t, bins.Agent, root, baseURL, clientID, clientSecret, redirectURI, statePath)
	authURL := waitForAuthURL(t, authURLCh, initProc)
	runRodneyOAuthFlow(t, authURL, "agentbot", e2eAgentPassword)
	waitForProcessExit(t, initProc, 30*time.Second)

	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file not created: %v", err)
	}
	state, err := loadAgentState(statePath)
	if err != nil {
		t.Fatalf("loadAgentState: %v", err)
	}
	if state.Mode != agentModeTestCounter {
		t.Fatalf("state mode mismatch: got %q want %q", state.Mode, agentModeTestCounter)
	}
	if strings.TrimSpace(state.Model) != "" {
		t.Fatalf("expected empty model in test-counter mode, got %q", state.Model)
	}
	if got := strings.TrimSpace(state.LMStudioURL); got != "" && got != "http://127.0.0.1:1234" {
		t.Fatalf("unexpected LM Studio URL in test-counter mode: %q", got)
	}

	runCommand(t, "", bins.Agent,
		"run",
		"--state-file", statePath,
		"--workspace", repoPath,
		"--verbosity", "0",
	)

	store, err := common.StoreInit(pgDSN, root)
	if err != nil {
		t.Fatalf("StoreInit: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	adminID := verifyUserID(t, store, "admin", e2eAdminPassword)
	agentID := verifyUserID(t, store, "agentbot", e2eAgentPassword)

	grant, found, err := store.GetLatestActiveOAuthConsentGrant(ctx, agentID, clientID)
	if err != nil {
		t.Fatalf("GetLatestActiveOAuthConsentGrant: %v", err)
	}
	if !found {
		t.Fatalf("expected oauth consent grant for client %q", clientID)
	}
	wantScopes := []string{"profile:read", "ticket:read", "ticket:write"}
	if grant.UserID != agentID {
		t.Fatalf("grant user mismatch: got %d want %d", grant.UserID, agentID)
	}
	if !reflect.DeepEqual(grant.RequestedScopes, wantScopes) {
		t.Fatalf("requested scopes mismatch: got %v want %v", grant.RequestedScopes, wantScopes)
	}
	if !reflect.DeepEqual(grant.GrantedScopes, wantScopes) {
		t.Fatalf("granted scopes mismatch: got %v want %v", grant.GrantedScopes, wantScopes)
	}

	comments, err := store.ListTicketCommentsForTicket(ctx, adminID, trackerSlug, ticketSlug)
	if err != nil {
		t.Fatalf("ListTicketCommentsForTicket: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 ticket comments, got %d", len(comments))
	}
	var ackComment, completionComment *common.TicketComment
	for i := range comments {
		if strings.Contains(comments[i].Content, "Agent acknowledged ticket") {
			ackComment = &comments[i]
		}
		if strings.Contains(comments[i].Content, "Agent completed ticket") {
			completionComment = &comments[i]
		}
	}
	if ackComment == nil {
		t.Fatalf("acknowledgement comment not found: %+v", comments)
	}
	if completionComment == nil {
		t.Fatalf("completion comment not found: %+v", comments)
	}
	if ackComment.AttributedTo != state.Profile.ActorID {
		t.Fatalf("ack comment actor mismatch: got %q want %q", ackComment.AttributedTo, state.Profile.ActorID)
	}
	if completionComment.AttributedTo != state.Profile.ActorID {
		t.Fatalf("completion comment actor mismatch: got %q want %q", completionComment.AttributedTo, state.Profile.ActorID)
	}
	for _, want := range []string{"test-step:1", "test-step:2", "test-step:3"} {
		if !strings.Contains(ackComment.Content, want) {
			t.Fatalf("ack comment missing progress update %q: %q", want, ackComment.Content)
		}
	}
	if !strings.Contains(completionComment.Content, "Agent completed ticket") {
		t.Fatalf("unexpected completion comment: %q", completionComment.Content)
	}

	ticketObj, found, err := store.GetLocalTicketObjectBySlug(ctx, trackerSlug, ticketSlug)
	if err != nil {
		t.Fatalf("GetLocalTicketObjectBySlug: %v", err)
	}
	if !found {
		t.Fatalf("expected ticket object for %s/%s", trackerSlug, ticketSlug)
	}
	wantCurrentContent := "Investigate deterministic e2e flow"
	if got := contentFromBodyJSON(t, ticketObj.BodyJSON); got != wantCurrentContent {
		t.Fatalf("ticket content mismatch: got %q want %q", got, wantCurrentContent)
	}

	versions, err := store.ListTicketCommentVersions(ctx, adminID, trackerSlug, ticketSlug, ackComment.Slug, 10)
	if err != nil {
		t.Fatalf("ListTicketCommentVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 comment versions, got %d", len(versions))
	}
	for i, want := range []string{"test-step:1\n\ntest-step:2", "test-step:1", "Agent acknowledged ticket"} {
		if got := contentFromBodyJSON(t, versions[i].BodyJSON); !strings.Contains(got, want) {
			t.Fatalf("version %d content mismatch: got %q to contain %q", i, got, want)
		}
	}

	assigned, err := store.ListAssignedTicketsForUser(ctx, agentID, common.AssignedTicketListOptions{Limit: 10, AgentCompletionPendingOnly: true})
	if err != nil {
		t.Fatalf("ListAssignedTicketsForUser: %v", err)
	}
	if len(assigned) != 0 {
		t.Fatalf("expected no completion-pending assigned tickets, got %d", len(assigned))
	}
}

func skipUnlessE2E(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	if strings.TrimSpace(os.Getenv("RUN_E2E")) == "" {
		t.Skip("set RUN_E2E=1 to run e2e tests")
	}
	adminDSN := strings.TrimSpace(os.Getenv("BV_E2E_PG_ADMIN_DSN"))
	if adminDSN == "" {
		t.Skip("set BV_E2E_PG_ADMIN_DSN to run e2e tests")
	}
	for _, tool := range []string{"rodney", "git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("required tool %q not available: %v", tool, err)
		}
	}
	for _, path := range []string{"cert/bv.local+3.pem", "cert/bv.local+3-key.pem"} {
		resolvedPath, err := repoRootPath(path)
		if err != nil {
			t.Fatalf("repoRootPath(%q): %v", path, err)
		}
		path = resolvedPath
		if _, err := os.Stat(path); err != nil {
			t.Skipf("required TLS file missing: %s (%v)", path, err)
		}
	}
	return adminDSN
}

func createTempDatabase(t *testing.T, adminDSN string) string {
	t.Helper()
	adminDB, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("sql.Open admin DSN: %v", err)
	}
	if err := adminDB.Ping(); err != nil {
		_ = adminDB.Close()
		t.Fatalf("admin database ping: %v", err)
	}

	dbName := fmt.Sprintf("bv_e2e_%d", time.Now().UnixNano())
	if _, err := adminDB.Exec(`CREATE DATABASE ` + quoteIdentifier(dbName)); err != nil {
		t.Fatalf("CREATE DATABASE %s: %v", dbName, err)
	}

	t.Cleanup(func() {
		_, _ = adminDB.Exec(
			`SELECT pg_terminate_backend(pid)
			   FROM pg_stat_activity
			  WHERE datname = $1
			    AND pid <> pg_backend_pid()`,
			dbName,
		)
		_, _ = adminDB.Exec(`DROP DATABASE IF EXISTS ` + quoteIdentifier(dbName))
		_ = adminDB.Close()
	})

	pgDSN, err := replaceDatabaseInDSN(adminDSN, dbName)
	if err != nil {
		t.Fatalf("replaceDatabaseInDSN: %v", err)
	}
	return pgDSN
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func reserveFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserveFreePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func writeRootConfig(t *testing.T, root string, port int) {
	t.Helper()
	cfg := common.RootConfig{
		Hostname: fmt.Sprintf("https://bv.local:%d", port),
		Port:     port,
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal root config: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(common.RootConfigPath(root), raw, 0o600); err != nil {
		t.Fatalf("write root config: %v", err)
	}
}

func createDummyRepo(t *testing.T, root, repoName string) (string, string) {
	t.Helper()
	repoPath := filepath.Join(common.RootRepoPath(root), repoName)
	if err := os.MkdirAll(repoPath, 0o700); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# e2e repo\n"), 0o600); err != nil {
		t.Fatalf("write repo README: %v", err)
	}
	runCommand(t, repoPath, "git", "init")
	return repoName, repoPath
}

type builtBinaries struct {
	Web   string
	Ctl   string
	Agent string
}

func repoRootDir(t *testing.T) string {
	t.Helper()
	root, err := repoRootDirFromWD()
	if err != nil {
		t.Fatalf("repoRootDirFromWD: %v", err)
	}
	return root
}

func buildE2EBinaries(t *testing.T, repoRoot string) builtBinaries {
	t.Helper()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll bin: %v", err)
	}

	webBin := filepath.Join(binDir, "BorealValley-web")
	ctlBin := filepath.Join(binDir, "BorealValley-ctl")
	agentBin := filepath.Join(binDir, "BorealValley-agent")
	runCommand(t, repoRoot, "go", "build", "-o", webBin, "./src/cmd/web")
	runCommand(t, repoRoot, "go", "build", "-o", ctlBin, "./src/cmd/ctl")
	runCommand(t, repoRoot, "go", "build", "-o", agentBin, "./src/cmd/agent")

	return builtBinaries{Web: webBin, Ctl: ctlBin, Agent: agentBin}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type processHandle struct {
	cmd    *exec.Cmd
	stdout *lockedBuffer
	stderr *lockedBuffer
	done   chan error
}

func startProcess(t *testing.T, dir string, bin string, args ...string) *processHandle {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	stdout := &lockedBuffer{}
	stderr := &lockedBuffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s %v: %v", bin, args, err)
	}
	handle := &processHandle{
		cmd:    cmd,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan error, 1),
	}
	go func() {
		handle.done <- cmd.Wait()
	}()
	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return
		}
		_ = cmd.Process.Kill()
		select {
		case <-handle.done:
		case <-time.After(5 * time.Second):
		}
	})
	return handle
}

func waitForHTTPSReady(t *testing.T, target string, proc *processHandle) {
	t.Helper()
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(target)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}

		select {
		case err := <-proc.done:
			t.Fatalf("web server exited before becoming ready: %v\nstdout:\n%s\nstderr:\n%s", err, proc.stdout.String(), proc.stderr.String())
		default:
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s\nstdout:\n%s\nstderr:\n%s", target, proc.stdout.String(), proc.stderr.String())
}

func runCommand(t *testing.T, dir string, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %s\nerr: %v\noutput:\n%s", bin, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func parseKeyValueOutput(t *testing.T, output, key string) string {
	t.Helper()
	prefix := key + "="
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("missing %s in output:\n%s", key, output)
	return ""
}

func newE2EWebClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func loginWebUser(t *testing.T, client *http.Client, baseURL, username, password string) {
	t.Helper()
	resp := postForm(t, client, baseURL+"/web/login", url.Values{
		"username": {username},
		"password": {password},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("login failed: status=%d body=%s", resp.StatusCode, body)
	}
}

func createAdminTaskFlow(t *testing.T, client *http.Client, baseURL, repoSlug, agentUsername string) (string, string) {
	t.Helper()

	resp := postForm(t, client, baseURL+"/web/repo/"+repoSlug+"/member", url.Values{
		"action":   {"add"},
		"username": {agentUsername},
	})
	assertRedirectStatus(t, resp, http.StatusSeeOther)

	resp = postForm(t, client, baseURL+"/web/ticket-tracker", url.Values{
		"name":    {"Agent Flow Tracker"},
		"summary": {"E2E tracker"},
	})
	trackerSlug := queryValueFromRedirect(t, resp, "created")

	resp = postForm(t, client, baseURL+"/web/repo/"+repoSlug+"/ticket-tracker", url.Values{
		"action":  {"assign"},
		"tracker": {trackerSlug},
	})
	assertRedirectStatus(t, resp, http.StatusSeeOther)

	resp = postForm(t, client, baseURL+"/web/ticket-tracker/"+trackerSlug+"/ticket", url.Values{
		"repo":     {repoSlug},
		"summary":  {"Deterministic E2E Ticket"},
		"priority": {"0"},
		"content":  {"Investigate deterministic e2e flow"},
	})
	ticketSlug := queryValueFromRedirect(t, resp, "created-ticket")

	resp = postForm(t, client, baseURL+"/web/ticket-tracker/"+trackerSlug+"/ticket/"+ticketSlug+"/assignee", url.Values{
		"action":   {"add"},
		"username": {agentUsername},
	})
	assertRedirectStatus(t, resp, http.StatusSeeOther)

	return trackerSlug, ticketSlug
}

func postForm(t *testing.T, client *http.Client, target string, vals url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(vals.Encode()))
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	u, _ := url.Parse(target)
	req.Header.Set("Origin", u.Scheme+"://"+u.Host)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do POST %s: %v", target, err)
	}
	return resp
}

func assertRedirectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: got %d want %d body=%s", resp.StatusCode, want, body)
	}
}

func queryValueFromRedirect(t *testing.T, resp *http.Response, key string) string {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: got %d body=%s", resp.StatusCode, body)
	}
	loc := strings.TrimSpace(resp.Header.Get("Location"))
	if loc == "" {
		t.Fatalf("redirect missing Location header")
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("url.Parse location %q: %v", loc, err)
	}
	value := strings.TrimSpace(u.Query().Get(key))
	if value == "" {
		t.Fatalf("redirect location %q missing query key %q", loc, key)
	}
	return value
}

func startAgentInit(t *testing.T, agentBin, root, baseURL, clientID, clientSecret, redirectURI, statePath string) (*processHandle, <-chan string) {
	t.Helper()
	cmd := exec.Command(agentBin,
		"init",
		"--server-url", baseURL,
		"--client-id", clientID,
		"--client-secret", clientSecret,
		"--redirect-uri", redirectURI,
		"--mode", agentModeTestCounter,
		"--state-file", statePath,
		"--no-open-browser",
		"--verbosity", "0",
	)
	cmd.Dir = root

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent init: %v", err)
	}

	stdout := &lockedBuffer{}
	authURLCh := make(chan string, 1)
	handle := &processHandle{
		cmd:    cmd,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan error, 1),
	}

	go func() {
		defer close(authURLCh)
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			_, _ = stdout.Write([]byte(line + "\n"))
			if strings.HasPrefix(strings.TrimSpace(line), "http://") || strings.HasPrefix(strings.TrimSpace(line), "https://") {
				select {
				case authURLCh <- strings.TrimSpace(line):
				default:
				}
			}
		}
	}()

	go func() {
		handle.done <- cmd.Wait()
	}()

	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return
		}
		_ = cmd.Process.Kill()
		select {
		case <-handle.done:
		case <-time.After(5 * time.Second):
		}
	})

	return handle, authURLCh
}

func waitForAuthURL(t *testing.T, authURLCh <-chan string, proc *processHandle) string {
	t.Helper()
	select {
	case authURL, ok := <-authURLCh:
		if !ok || strings.TrimSpace(authURL) == "" {
			t.Fatalf("agent init exited before printing authorize URL\nstdout:\n%s\nstderr:\n%s", proc.stdout.String(), proc.stderr.String())
		}
		return authURL
	case err := <-proc.done:
		t.Fatalf("agent init exited before printing authorize URL: %v\nstdout:\n%s\nstderr:\n%s", err, proc.stdout.String(), proc.stderr.String())
	case <-time.After(20 * time.Second):
		t.Fatalf("timed out waiting for authorize URL\nstdout:\n%s\nstderr:\n%s", proc.stdout.String(), proc.stderr.String())
	}
	return ""
}

func runRodneyOAuthFlow(t *testing.T, authURL, username, password string) {
	t.Helper()
	rodneyDir := filepath.Join(t.TempDir(), "rodney")
	if err := os.MkdirAll(rodneyDir, 0o700); err != nil {
		t.Fatalf("MkdirAll rodney dir: %v", err)
	}

	runRodney(t, rodneyDir, "start", "--local")
	t.Cleanup(func() {
		cmd := exec.Command("rodney", "stop")
		cmd.Dir = rodneyDir
		_, _ = cmd.CombinedOutput()
	})

	runRodney(t, rodneyDir, "open", authURL)
	runRodney(t, rodneyDir, "waitload")
	runRodney(t, rodneyDir, "waitstable")
	runRodney(t, rodneyDir, "input", "input[name='username']", username)
	runRodney(t, rodneyDir, "input", "input[name='password']", password)
	runRodney(t, rodneyDir, "click", "button[type='submit']")
	runRodney(t, rodneyDir, "waitload")
	runRodney(t, rodneyDir, "waitstable")

	for _, scope := range []string{"profile:read", "ticket:read", "ticket:write"} {
		runRodney(t, rodneyDir, "click", fmt.Sprintf("input[name='scope'][value='%s']", scope))
	}
	runRodney(t, rodneyDir, "click", "button[name='action'][value='approve']")
	runRodney(t, rodneyDir, "waitload")
	runRodney(t, rodneyDir, "waitstable")
	runRodney(t, rodneyDir, "assert", "document.body.innerText.includes('Authorization complete')")
}

func runRodney(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("rodney", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rodney %s failed: %v\noutput:\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func waitForProcessExit(t *testing.T, proc *processHandle, timeout time.Duration) {
	t.Helper()
	select {
	case err := <-proc.done:
		if err != nil {
			t.Fatalf("process exited with error: %v\nstdout:\n%s\nstderr:\n%s", err, proc.stdout.String(), proc.stderr.String())
		}
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for process exit\nstdout:\n%s\nstderr:\n%s", proc.stdout.String(), proc.stderr.String())
	}
}

func verifyUserID(t *testing.T, store *common.Store, username, password string) int64 {
	t.Helper()
	userID, ok, err := store.VerifyUser(context.Background(), username, password)
	if err != nil {
		t.Fatalf("VerifyUser(%s): %v", username, err)
	}
	if !ok {
		t.Fatalf("VerifyUser(%s): expected success", username)
	}
	return userID
}

func contentFromBodyJSON(t *testing.T, raw []byte) string {
	t.Helper()
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal object body: %v", err)
	}
	value, ok := body["content"]
	if !ok {
		t.Fatalf("object body missing content field")
	}
	content, ok := value.(string)
	if !ok {
		t.Fatalf("object body content has unexpected type %T", value)
	}
	return content
}
