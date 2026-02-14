# Add License Comments Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a dev script that reads `LICENSE.md` and prepends it as `//` comments to every `.go` file, while skipping files that already have a top header comment and reporting those skips to stderr.

**Architecture:** A small standalone Go program in `tools/dev/` handles repo walking and file rewriting. The core logic is separated into small functions so tests can cover build-tag preservation, existing-header detection, idempotence, and the `.go`-only file walk behavior.

**Tech Stack:** Go, standard library file I/O and filepath walking, `go test`.

---

### Task 1: Add failing tests for header rewriting

**Files:**
- Create: `tools/dev/add-license_test.go`
- Create: `tools/dev/testdata/` only if fixtures become necessary
- Modify: `LICENSE.md` should remain unchanged

**Step 1: Write the failing test**

Add table-driven tests for a function that rewrites a single Go file body:

- plain `package main` file gets the generated license block prepended;
- file starting with `//go:build ...` keeps the build tags first in the file content after the inserted license block;
- file with an existing ordinary top comment is skipped and reported as skipped;
- file already containing the generated license block is unchanged and not reported as skipped.

**Step 2: Run test to verify it fails**

Run: `go test ./tools/dev -run TestApplyLicenseToSource`

Expected: FAIL because the script implementation does not exist yet.

**Step 3: Write minimal implementation**

Create `tools/dev/add-license.go` with package-level helpers for:

- converting `LICENSE.md` text into a `//` comment block;
- applying that block to one file's contents;
- identifying leading build tags and ordinary header comments.

**Step 4: Run test to verify it passes**

Run: `go test ./tools/dev -run TestApplyLicenseToSource`

Expected: PASS.

**Step 5: Commit**

```bash
git add docs/plans/2026-03-17-add-license-comments-design.md docs/plans/2026-03-17-add-license-comments.md tools/dev/add-license.go tools/dev/add-license_test.go
git commit -m "add license comment script"
```

---

### Task 2: Add failing tests for repo walking and file selection

**Files:**
- Modify: `tools/dev/add-license_test.go`
- Modify: `tools/dev/add-license.go`

**Step 1: Write the failing test**

Add a temp-directory integration-style test that creates:

- one `.go` file needing a license;
- one `.go` file with an existing header comment;
- one non-Go file.

Assert that:

- only `.go` files are considered;
- the plain `.go` file is rewritten;
- the header-comment `.go` file is unchanged;
- the skipped file path is written to stderr output.

**Step 2: Run test to verify it fails**

Run: `go test ./tools/dev -run TestProcessTree`

Expected: FAIL because directory walking and stderr reporting are incomplete.

**Step 3: Write minimal implementation**

Extend `tools/dev/add-license.go` with:

- a directory walk rooted at the current working directory;
- `.go`-only filtering;
- stderr reporting for skipped files;
- in-place file rewrites.

**Step 4: Run test to verify it passes**

Run: `go test ./tools/dev -run TestProcessTree`

Expected: PASS.

**Step 5: Commit**

```bash
git add tools/dev/add-license.go tools/dev/add-license_test.go
git commit -m "test license script walk"
```

---

### Task 3: Verify the script end to end

**Files:**
- Modify: `tools/dev/add-license.go` if polish is needed
- Modify: `tools/dev/add-license_test.go` if gaps are found

**Step 1: Run focused tests**

Run:

```bash
go test ./tools/dev
```

Expected: PASS.

**Step 2: Run repo-wide tests**

Run:

```bash
go test ./...
```

Expected: PASS.

**Step 3: Cross-check script usage**

Run:

```bash
go run ./tools/dev/add-license.go
```

Expected: the command processes `.go` files only, rewrites eligible files, and prints skipped paths with existing top comments to stderr.

**Step 4: Commit**

```bash
git add tools/dev/add-license.go tools/dev/add-license_test.go
git commit -m "verify license script"
```
