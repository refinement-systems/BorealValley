# Task Endpoint Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `/data/{id}/task` endpoints that create a task directory in the project repo, commit and push it, and set up a git branch + worktree.

**Architecture:** Git helpers live in `src/internal/common/git.go` (using `os/exec`). The task handler in `src/cmd/web/task.go` follows the same pattern as `chat.go`. Identifiers are 12 random bytes encoded with `base64.RawURLEncoding` (16 chars, URL-safe, no padding).

**Tech Stack:** Go stdlib (`os/exec`, `crypto/rand`, `encoding/base64`, `encoding/json`), existing `common.ControlPlane`, existing LM Studio model-listing helpers.

---

### Task 1: Remove tasks table references

**Files:**
- Modify: `src/internal/assets/sql/create.sql`
- Modify: `src/internal/common/control_projects_test.go`

**Step 1: Remove the DROP TABLE line from create.sql**

In `src/internal/assets/sql/create.sql`, delete this line:

```sql
DROP TABLE IF EXISTS tasks;
```

The file should end with the closing `;` of the projects table.

**Step 2: Remove the stale test**

In `src/internal/common/control_projects_test.go`, delete the entire `TestControlSchemaDoesNotCreateTasksTable` function (lines 9–23).

**Step 3: Run tests to confirm nothing broke**

```bash
go test ./...
```

Expected: all tests pass.

**Step 4: Commit**

```bash
git add src/internal/assets/sql/create.sql src/internal/common/control_projects_test.go
git commit -m "remove tasks table references"
```

---

### Task 2: newTaskID helper

**Files:**
- Create: `src/internal/common/task_id.go`
- Create: `src/internal/common/task_id_test.go`

**Step 1: Write the failing test**

Create `src/internal/common/task_id_test.go`:

```go
package common

import (
	"testing"
)

func TestNewTaskID(t *testing.T) {
	id := NewTaskID()
	if len(id) != 16 {
		t.Fatalf("expected 16 chars, got %d: %q", len(id), id)
	}
	for _, c := range id {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			t.Fatalf("non-URL-safe char %q in id %q", c, id)
		}
	}

	// Two IDs should differ (birthday-paradox safe at 96 bits).
	id2 := NewTaskID()
	if id == id2 {
		t.Fatal("two NewTaskID calls returned the same value")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./src/internal/common -run TestNewTaskID -v
```

Expected: FAIL — `NewTaskID` undefined.

**Step 3: Write minimal implementation**

Create `src/internal/common/task_id.go`:

```go
package common

import (
	"crypto/rand"
	"encoding/base64"
)

// NewTaskID returns a random 96-bit identifier encoded as a 16-character
// URL-safe base64 string (no padding).
func NewTaskID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./src/internal/common -run TestNewTaskID -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add src/internal/common/task_id.go src/internal/common/task_id_test.go
git commit -m "add NewTaskID helper"
```

---

### Task 3: IsGitRepo helper

**Files:**
- Create: `src/internal/common/git.go`
- Create: `src/internal/common/git_test.go`

**Step 1: Write the failing test**

Create `src/internal/common/git_test.go`:

```go
package common

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsGitRepo(t *testing.T) {
	dir := t.TempDir()

	if IsGitRepo(dir) {
		t.Fatal("expected plain dir to not be a git repo")
	}

	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	if !IsGitRepo(dir) {
		t.Fatal("expected git-init'd dir to be a git repo")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./src/internal/common -run TestIsGitRepo -v
```

Expected: FAIL — `IsGitRepo` undefined.

**Step 3: Write minimal implementation**

Create `src/internal/common/git.go`:

```go
package common

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsGitRepo reports whether dir contains a git repository (i.e. has a .git entry).
func IsGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// CommitAndPush stages all files under .task/, commits with message, and pushes
// to origin main. If the push is rejected because the local branch is behind
// upstream, it fetches and rebases then retries the push once.
func CommitAndPush(dir, message string) error {
	if err := gitRun(dir, "add", ".task/"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if err := gitRun(dir, "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	pushErr := gitRun(dir, "push", "origin", "main")
	if pushErr == nil {
		return nil
	}
	// Push failed — try fetch + rebase then retry.
	if err := gitRun(dir, "fetch", "origin"); err != nil {
		return fmt.Errorf("git fetch: %w (original push error: %v)", err, pushErr)
	}
	if err := gitRun(dir, "rebase", "origin/main"); err != nil {
		return fmt.Errorf("git rebase: %w", err)
	}
	if err := gitRun(dir, "push", "origin", "main"); err != nil {
		return fmt.Errorf("git push (after rebase): %w", err)
	}
	return nil
}

// CreateBranchAndWorktree creates a new branch in repoDir and a git worktree
// for it at worktreeDir.
func CreateBranchAndWorktree(repoDir, branch, worktreeDir string) error {
	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0700); err != nil {
		return fmt.Errorf("mkdir worktree parent: %w", err)
	}
	if err := gitRun(repoDir, "branch", branch); err != nil {
		return fmt.Errorf("git branch: %w", err)
	}
	if err := gitRun(repoDir, "worktree", "add", worktreeDir, branch); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	return nil
}

// gitRun runs a git command in dir, returning a descriptive error that includes
// stderr output if the command fails.
func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./src/internal/common -run TestIsGitRepo -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add src/internal/common/git.go src/internal/common/git_test.go
git commit -m "add IsGitRepo git helper"
```

---

### Task 4: CommitAndPush helper tests

**Files:**
- Modify: `src/internal/common/git_test.go`

**Step 1: Add a helper and test for CommitAndPush**

Append to `src/internal/common/git_test.go`:

```go
// initBareAndClone creates a bare "remote" repo and clones it into a working
// directory, returning (bareDir, cloneDir). Both are inside t.TempDir().
func initBareAndClone(t *testing.T) (bareDir, cloneDir string) {
	t.Helper()
	root := t.TempDir()
	bareDir = filepath.Join(root, "bare.git")
	cloneDir = filepath.Join(root, "clone")

	if err := exec.Command("git", "init", "--bare", bareDir).Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}
	if err := exec.Command("git", "clone", bareDir, cloneDir).Run(); err != nil {
		t.Fatalf("git clone: %v", err)
	}
	// Configure identity for commits inside the clone.
	gitConfig(t, cloneDir, "user.email", "test@example.com")
	gitConfig(t, cloneDir, "user.name", "Test")
	// Create an initial commit so main exists.
	seed := filepath.Join(cloneDir, "seed")
	if err := os.WriteFile(seed, []byte("seed\n"), 0600); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	if err := exec.Command("git", "-C", cloneDir, "add", "seed").Run(); err != nil {
		t.Fatalf("git add seed: %v", err)
	}
	if err := exec.Command("git", "-C", cloneDir, "commit", "-m", "init").Run(); err != nil {
		t.Fatalf("git commit init: %v", err)
	}
	if err := exec.Command("git", "-C", cloneDir, "push", "origin", "main").Run(); err != nil {
		// Try "master" if "main" doesn't exist yet.
		_ = exec.Command("git", "-C", cloneDir, "push", "origin", "HEAD:main").Run()
	}
	return bareDir, cloneDir
}

func gitConfig(t *testing.T, dir, key, val string) {
	t.Helper()
	if err := exec.Command("git", "-C", dir, "config", key, val).Run(); err != nil {
		t.Fatalf("git config %s: %v", key, err)
	}
}

func TestCommitAndPush(t *testing.T) {
	_, cloneDir := initBareAndClone(t)

	taskDir := filepath.Join(cloneDir, ".task", "AAAAAAAAAAAAAAAA")
	if err := os.MkdirAll(taskDir, 0700); err != nil {
		t.Fatalf("mkdir task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "root.json"), []byte(`{}`), 0600); err != nil {
		t.Fatalf("write root.json: %v", err)
	}

	if err := CommitAndPush(cloneDir, "[task] AAAAAAAAAAAAAAAA"); err != nil {
		t.Fatalf("CommitAndPush: %v", err)
	}

	// Verify the commit message appears in git log.
	out, err := exec.Command("git", "-C", cloneDir, "log", "--oneline", "-1").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(out), "[task] AAAAAAAAAAAAAAAA") {
		t.Fatalf("expected commit message in log, got: %s", out)
	}
}
```

Also add `"strings"` to the import block if not already present.

**Step 2: Run test to verify it passes**

```bash
go test ./src/internal/common -run TestCommitAndPush -v
```

Expected: PASS.

**Step 3: Commit**

```bash
git add src/internal/common/git_test.go
git commit -m "test CommitAndPush"
```

---

### Task 5: CreateBranchAndWorktree helper tests

**Files:**
- Modify: `src/internal/common/git_test.go`

**Step 1: Add test for CreateBranchAndWorktree**

Append to `src/internal/common/git_test.go`:

```go
func TestCreateBranchAndWorktree(t *testing.T) {
	_, cloneDir := initBareAndClone(t)
	worktreeDir := filepath.Join(t.TempDir(), "wt", "proj1", "AAAAAAAAAAAAAAAA")

	if err := CreateBranchAndWorktree(cloneDir, "task-AAAAAAAAAAAAAAAA", worktreeDir); err != nil {
		t.Fatalf("CreateBranchAndWorktree: %v", err)
	}

	// Worktree directory should exist and be a git repo.
	if !IsGitRepo(worktreeDir) {
		t.Fatal("expected worktree dir to be a git repo")
	}

	// The branch should be checked out in the worktree.
	out, err := exec.Command("git", "-C", worktreeDir, "branch", "--show-current").Output()
	if err != nil {
		t.Fatalf("git branch: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "task-AAAAAAAAAAAAAAAA" {
		t.Fatalf("expected branch task-AAAAAAAAAAAAAAAA, got %q", got)
	}
}
```

**Step 2: Run test to verify it passes**

```bash
go test ./src/internal/common -run TestCreateBranchAndWorktree -v
```

Expected: PASS.

**Step 3: Run all common tests**

```bash
go test ./src/internal/common/... -v
```

Expected: all pass.

**Step 4: Commit**

```bash
git add src/internal/common/git_test.go
git commit -m "test CreateBranchAndWorktree"
```

---

### Task 6: ctl-task.html template

**Files:**
- Create: `src/internal/assets/html/ctl-task.html`
- Modify: `src/internal/assets/assets.go`

**Step 1: Create the template**

Create `src/internal/assets/html/ctl-task.html`:

```html
<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
  </head>
  <body>
    <h1>New Task</h1>
    {{if .Err}}<p style="color:red">{{.Err}}</p>{{end}}
    {{if .TaskLink}}
    <p>Task created: <a href="{{.TaskLink}}">{{.TaskLink}}</a></p>
    {{else}}
    <form method="POST" action="/data/{{.ProjectID}}/task">
      <label>Model:
        <select name="model">
          {{range .Models}}
          <option value="{{.Key}}">{{.Name}}</option>
          {{end}}
        </select>
      </label>
      <br>
      <label>Prompt:<br>
        <textarea name="prompt" rows="8" cols="70"></textarea>
      </label>
      <br>
      <button type="submit">Create Task</button>
    </form>
    {{end}}
    <p><a href="/data/{{.ProjectID}}">Project</a> | <a href="/">Home</a></p>
  </body>
</html>
```

**Step 2: Register the embed**

In `src/internal/assets/assets.go`, add after `HtmlCtlChat`:

```go
//go:embed html/ctl-task.html
var HtmlCtlTask string
```

**Step 3: Build to confirm no compile errors**

```bash
just build-web
```

Expected: compiles cleanly.

**Step 4: Commit**

```bash
git add src/internal/assets/html/ctl-task.html src/internal/assets/assets.go
git commit -m "add ctl-task template"
```

---

### Task 7: Task handler

**Files:**
- Create: `src/cmd/web/task.go`
- Create: `src/cmd/web/task_test.go`

**Step 1: Write the failing tests**

Create `src/cmd/web/task_test.go`:

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/refinement-systems/BorealValley/src/internal/common"
)

func TestDataTaskGetRendersForm(t *testing.T) {
	cp := newTestControlPlane(t)
	dir := t.TempDir()
	if err := cp.CreateProject(context.Background(), dir); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	app := &application{controlPlane: cp}
	req := httptest.NewRequest(http.MethodGet, "/data/1/task", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	app.dataTask(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d\n%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `action="/data/1/task"`) {
		t.Fatalf("expected form action in body:\n%s", rr.Body.String())
	}
}

func TestDataTaskPostNotGitRepo(t *testing.T) {
	cp := newTestControlPlane(t)
	dir := t.TempDir() // plain dir, no git
	if err := cp.CreateProject(context.Background(), dir); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	app := &application{controlPlane: cp}
	req := httptest.NewRequest(http.MethodPost, "/data/1/task",
		strings.NewReader("model=m&prompt=p"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	app.dataTask(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "not a git repository") {
		t.Fatalf("expected git error message, got:\n%s", rr.Body.String())
	}
}

func TestDataTaskPostCreatesTaskAndWorktree(t *testing.T) {
	cp := newTestControlPlane(t)

	// Set up a bare remote + clone to act as the project repo.
	root := t.TempDir()
	bareDir := filepath.Join(root, "bare.git")
	cloneDir := filepath.Join(root, "clone")

	if err := exec.Command("git", "init", "--bare", bareDir).Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}
	if err := exec.Command("git", "clone", bareDir, cloneDir).Run(); err != nil {
		t.Fatalf("git clone: %v", err)
	}
	gitCfg := func(key, val string) {
		exec.Command("git", "-C", cloneDir, "config", key, val).Run()
	}
	gitCfg("user.email", "test@example.com")
	gitCfg("user.name", "Test")

	// Seed initial commit so main branch exists.
	seedFile := filepath.Join(cloneDir, "seed")
	if err := writeFileAbs(seedFile, "seed"); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	exec.Command("git", "-C", cloneDir, "add", "seed").Run()
	exec.Command("git", "-C", cloneDir, "commit", "-m", "init").Run()
	exec.Command("git", "-C", cloneDir, "push", "origin", "HEAD:main").Run()

	if err := cp.CreateProject(context.Background(), cloneDir); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Override EnvDirState to a temp dir via env var.
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))

	app := &application{controlPlane: cp}
	req := httptest.NewRequest(http.MethodPost, "/data/1/task",
		strings.NewReader("model=mymodel&prompt=hello+world"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	app.dataTask(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d\n%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "color:red") {
		t.Fatalf("unexpected error in response:\n%s", body)
	}
	if !strings.Contains(body, "/data/1/") {
		t.Fatalf("expected task link in response:\n%s", body)
	}
}

// writeFileAbs is a helper to write a file by absolute path.
func writeFileAbs(path, content string) error {
	return os.WriteFile(path, []byte(content), 0600)
}
```

Also add `"os"` to imports.

**Step 2: Run tests to verify they fail**

```bash
go test ./src/cmd/web -run "TestDataTask" -v
```

Expected: FAIL — `dataTask` undefined.

**Step 3: Write the implementation**

Create `src/cmd/web/task.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hypernetix/lmstudio-go/pkg/lmstudio"

	"github.com/refinement-systems/BorealValley/src/internal/assets"
	"github.com/refinement-systems/BorealValley/src/internal/common"
)

type taskCtlData struct {
	ProjectID int64
	Models    []modelRow
	Err       string
	TaskLink  string
}

var taskTmpl = template.Must(template.New("ctl-task").Parse(assets.HtmlCtlTask))

func (app *application) dataTask(w http.ResponseWriter, r *http.Request) {
	project, found, err := app.projectFromPathValue(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		app.taskCtlGet(w, r, project)
	case http.MethodPost:
		app.taskCtlPost(w, r, project)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app *application) taskCtlGet(w http.ResponseWriter, r *http.Request, project common.Project) {
	data := taskCtlData{ProjectID: project.ID}
	data.Models = listModels(&data.Err)
	renderTaskCtl(w, data)
}

func (app *application) taskCtlPost(w http.ResponseWriter, r *http.Request, project common.Project) {
	data := taskCtlData{ProjectID: project.ID}

	if err := r.ParseForm(); err != nil {
		data.Err = "bad form"
		renderTaskCtl(w, data)
		return
	}
	model := r.PostFormValue("model")
	prompt := r.PostFormValue("prompt")
	if model == "" || prompt == "" {
		data.Err = "model and prompt are required"
		renderTaskCtl(w, data)
		return
	}

	if !common.IsGitRepo(project.Path) {
		data.Err = "project directory is not a git repository"
		renderTaskCtl(w, data)
		return
	}

	taskNumber := common.NewTaskID()
	taskDir := filepath.Join(project.Path, ".task", taskNumber)
	if err := os.Mkdir(taskDir, 0700); err != nil {
		if os.IsExist(err) {
			data.Err = "task directory already exists; please retry"
		} else {
			data.Err = "failed to create task directory: " + err.Error()
		}
		renderTaskCtl(w, data)
		return
	}

	rootID := common.NewTaskID()
	rootFile := filepath.Join(taskDir, "root."+rootID+".json")
	payload, _ := json.Marshal(struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}{Model: model, Prompt: prompt})
	if err := os.WriteFile(rootFile, payload, 0600); err != nil {
		data.Err = "failed to write task file: " + err.Error()
		renderTaskCtl(w, data)
		return
	}

	commitMsg := "[task] " + taskNumber
	if err := common.CommitAndPush(project.Path, commitMsg); err != nil {
		data.Err = "git commit/push failed: " + err.Error()
		renderTaskCtl(w, data)
		return
	}

	worktreeDir := filepath.Join(
		common.EnvDirState(),
		".worktree",
		fmt.Sprintf("%d", project.ID),
		taskNumber,
	)
	branch := "task-" + taskNumber
	if err := common.CreateBranchAndWorktree(project.Path, branch, worktreeDir); err != nil {
		data.Err = "failed to create worktree: " + err.Error()
		renderTaskCtl(w, data)
		return
	}

	data.TaskLink = fmt.Sprintf("/data/%d/%s", project.ID, taskNumber)
	renderTaskCtl(w, data)
}

// listModels loads available LLM models from LM Studio, setting errMsg on failure.
func listModels(errMsg *string) []modelRow {
	addr, err := lmstudioDiscover()
	if err != nil {
		*errMsg = "LM Studio not found: " + err.Error()
		return nil
	}
	log := lmstudio.NewLogger(lmstudio.LogLevelError)
	client := lmstudio.NewLMStudioClient(addr, log)
	defer client.Close()

	loaded, err := client.ListAllLoadedModels()
	if err != nil {
		*errMsg = "failed to list models: " + err.Error()
		return nil
	}
	var rows []modelRow
	for _, m := range loaded {
		if m.Type != "" && m.Type != "llm" {
			continue
		}
		id := m.Identifier
		if id == "" {
			id = m.ModelKey
		}
		name := m.DisplayName
		if name == "" {
			name = m.ModelName
		}
		if name == "" {
			name = id
		}
		rows = append(rows, modelRow{Name: name, Key: id})
	}
	return rows
}

func renderTaskCtl(w http.ResponseWriter, data taskCtlData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = taskTmpl.Execute(w, data)
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./src/cmd/web -run "TestDataTask" -v
```

Expected: PASS (TestDataTaskGetRendersForm and TestDataTaskPostNotGitRepo pass; TestDataTaskPostCreatesTaskAndWorktree may be skipped if LM Studio absent — that's fine).

**Step 5: Run all web tests**

```bash
go test ./src/cmd/web/...
```

Expected: all pass.

**Step 6: Commit**

```bash
git add src/cmd/web/task.go src/cmd/web/task_test.go
git commit -m "add task handler"
```

---

### Task 8: Wire up route and update project page

**Files:**
- Modify: `src/cmd/web/main.go`
- Modify: `src/internal/assets/html/data-project.html`

**Step 1: Add route**

In `src/cmd/web/main.go`, after line 121 (`mux.HandleFunc("POST /data/{id}/chat/reset", ...)`), add:

```go
mux.HandleFunc("/data/{id}/task", app.requireAuth(app.dataTask))
```

**Step 2: Add "New Task" link to the project template**

In `src/internal/assets/html/data-project.html`, change the `<ul>` block from:

```html
    <ul>
      <li><a href="/data/{{.ID}}/chat">Chat</a></li>
    </ul>
```

to:

```html
    <ul>
      <li><a href="/data/{{.ID}}/chat">Chat</a></li>
      <li><a href="/data/{{.ID}}/task">New Task</a></li>
    </ul>
```

**Step 3: Update the auth-required routes test**

In `src/cmd/web/data_test.go`, `TestNewHandlerDataRoutesRequireAuth` currently checks `/data`, `/data/1`, `/data/1/chat`. Add `/data/1/task` to that list:

```go
for _, path := range []string{"/data", "/data/1", "/data/1/chat", "/data/1/task"} {
```

**Step 4: Run all tests**

```bash
go test ./...
```

Expected: all pass.

**Step 5: Build**

```bash
just build
```

Expected: compiles cleanly.

**Step 6: Commit**

```bash
git add src/cmd/web/main.go src/internal/assets/html/data-project.html src/cmd/web/data_test.go
git commit -m "wire task route and add project page link"
```

---

## Summary of new/modified files

| File | Change |
|------|--------|
| `src/internal/assets/sql/create.sql` | Remove `DROP TABLE IF EXISTS tasks` |
| `src/internal/common/control_projects_test.go` | Remove stale tasks-table test |
| `src/internal/common/task_id.go` | New — `NewTaskID()` |
| `src/internal/common/task_id_test.go` | New — tests for `NewTaskID` |
| `src/internal/common/git.go` | New — `IsGitRepo`, `CommitAndPush`, `CreateBranchAndWorktree` |
| `src/internal/common/git_test.go` | New — tests for all three |
| `src/internal/assets/html/ctl-task.html` | New — task form template |
| `src/internal/assets/assets.go` | Add `HtmlCtlTask` embed |
| `src/cmd/web/task.go` | New — `dataTask` handler |
| `src/cmd/web/task_test.go` | New — handler tests |
| `src/cmd/web/main.go` | Add `/data/{id}/task` route |
| `src/internal/assets/html/data-project.html` | Add "New Task" link |
| `src/cmd/web/data_test.go` | Add `/data/1/task` to auth test |
