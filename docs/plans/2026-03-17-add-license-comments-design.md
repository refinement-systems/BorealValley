# Design: Add license comments to Go source files

**Date:** 2026-03-17

## Goal

Add a development script under `tools/dev/` that reads `LICENSE.md` and prepends it as `//` comments to every `.go` file in the repository that does not already have a top-of-file comment.

## Scope

- Process only `.go` files.
- Source license text only from `LICENSE.md`.
- Modify files in place.
- Skip files that already have a top comment before the `package` clause and report those skipped file paths to stderr.
- Preserve Go build constraints by inserting the license above leading `//go:build` and `// +build` lines.
- Leave already-licensed files unchanged so the script is idempotent.

## Approach

Implement a small Go program at `tools/dev/add-license.go` and run it with `go run ./tools/dev/add-license.go`.

The script will:

1. Read `LICENSE.md`.
2. Convert the license into a Go line-comment block.
3. Walk the repository tree from the current working directory.
4. For each `.go` file:
   - detect and preserve any leading build-constraint block;
   - detect whether the file already starts with a non-build comment block and, if so, skip it and report the path to stderr;
   - detect whether the exact generated license block is already present and, if so, leave the file unchanged;
   - otherwise, prepend the license block and write the file back.

## Alternatives Considered

### Shell script with `find` and `sed`

Pros: small and quick to write.

Cons: fragile around build tags, blank-line handling, comment detection, and portable in-place edits.

### Go script using text parsing

Pros: robust, testable, portable across macOS/Linux, easy to keep idempotent.

Cons: slightly more code.

Recommended: Go script.

## Error Handling

- Fail fast if `LICENSE.md` cannot be read.
- Fail fast on directory-walk or file-write errors.
- Report skipped files with existing header comments to stderr without failing the run.

## Testing

Add table-driven tests next to the script covering:

- a plain file starting with `package`;
- a file with leading build constraints;
- a file with an existing header comment that must be skipped;
- a file already containing the generated license block;
- repo walking that touches only `.go` files.
