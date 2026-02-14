# Task Endpoint Design

Date: 2026-02-23

## Overview

Add a `/data/$ID/task` endpoint that creates a new task in the project repository: writes task metadata to the filesystem, commits and pushes it to main, and sets up a git branch and worktree for the task.

## Identifiers

A task identifier is 12 random bytes encoded with `base64.RawURLEncoding` (URL-safe, no padding), yielding a 16-character string. Both `$TASK_NUMBER` and `$ROOTID` use this format.

## File Layout

Written into the project repository:

```
$DIR/
  .task/
    $TASK_NUMBER/
      root.$ROOTID.json   ← {"model": "...", "prompt": "..."}
```

Worktree lives in the application state directory:

```
EnvDirState()/.worktree/$PROJECT_ID/$TASK_NUMBER/
```

## Git Helpers (`src/internal/common/git.go`)

Three functions using `os/exec` with the project directory as working dir:

- `IsGitRepo(dir string) bool` — checks for existence of `$dir/.git`
- `CommitAndPush(dir, message string) error` — `git add .task/`, `git commit`, `git push origin main`. If push fails because local is behind, `git fetch origin` + `git rebase origin/main`, retry push once. Other failures or post-rebase conflicts return error.
- `CreateBranchAndWorktree(repoDir, branch, worktreeDir string) error` — `git branch <branch>`, `git worktree add <worktreeDir> <branch>`, creating parent dirs as needed.

## Request Handling (`src/cmd/web/task.go`)

`dataTask()` routes GET and POST.

**GET /data/{id}/task:**
1. Fetch project via `projectFromPathValue`
2. Load available models from LM Studio
3. Render `ctl-task.html` with `{ProjectID, Models, Err}`

**POST /data/{id}/task** (abort on error, no rollback):
1. Fetch project; parse form (model, prompt)
2. `IsGitRepo(project.Path)` → error if false
3. Generate `taskNumber` (12 random bytes, `RawURLEncoding`)
4. `os.Mkdir("$DIR/.task/$TASK_NUMBER", 0700)` — fails if already exists
5. Generate `rootID`; write `root.$ROOTID.json`
6. `CommitAndPush(dir, "[task] "+taskNumber)`
7. `CreateBranchAndWorktree(dir, "task-"+taskNumber, EnvDirState()+"/.worktree/"+projectID+"/"+taskNumber)`
8. Respond with a page showing a link to `/data/{id}/{taskNumber}` (that URL 404s for now)

## Templates and Routing

**`ctl-task.html`:** Model selector, prompt textarea, submit button, error display. No SSE, no history. Posts to `/data/{{.ProjectID}}/task`.

**Route added to `main.go`:**
```go
mux.HandleFunc("/data/{id}/task", app.requireAuth(app.dataTask))
```

**`data-project.html`:** Add a "New Task" link to `/data/{{.ID}}/task`.

## Cleanup

- Remove `DROP TABLE IF EXISTS tasks` from `create.sql`
- Remove any other `tasks` references in Go code
