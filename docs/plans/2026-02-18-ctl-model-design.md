# Design: /ctl/model endpoint

Date: 2026-02-18

## Summary

Add a `GET /ctl/model` endpoint to `src/cmd/web` that discovers a local LM Studio
server, queries its downloaded and loaded models, merges the two lists, and renders
the result as a static HTML page. The endpoint is protected by the existing
`requireAuth` middleware.

## Files

| File | Change |
|------|--------|
| `src/cmd/web/lmstudio.go` | New — handler + merge logic |
| `src/cmd/web/lmstudio_test.go` | New — unit tests for merge logic and error path |
| `src/internal/assets/html/ctl-model.html` | New — page template |
| `src/internal/assets/assets.go` | Add `//go:embed html/ctl-model.html` |
| `src/cmd/web/main.go` | Register `/ctl/model` route |

## Data flow

1. `lmstudio.DiscoverLMStudioServer("", 0, logger)` — find server address.
   On failure: render template with `Err` set, stop.
2. `lmstudio.NewLMStudioClient(addr, logger)` — create client, defer `Close()`.
3. Concurrently call `ListDownloadedModels()` and `ListAllLoadedModels()` via
   goroutines. Either failure is treated as an empty list; the error is surfaced
   in `Err` alongside whatever partial results the other call returned.
4. Merge: iterate downloaded list first; for each entry whose `ModelKey` appears
   in the loaded list, set `IsLoaded = true`. Then append any loaded models whose
   key was not in the downloaded list.
5. Render `ctl-model.html` with:

```go
type modelCtlData struct {
    Models []*lmstudio.Model
    Err    string
}
```

## Template / UI

Plain HTML table, no CSS framework:

| Column | Source field |
|--------|-------------|
| Name | `ModelName` (fallback: `ModelKey`) |
| Type | `Type` |
| Format | `Format` |
| Size | Human-readable bytes (formatted in Go) |
| Loaded | `Yes` / `—` |

Error case: red `<p>` with `Err` text, no table rendered.
Link back to `/` in the footer.

## Error handling

| Condition | Behaviour |
|-----------|-----------|
| Discovery fails | `Err = "LM Studio not found: <err>"`, empty model list, HTTP 200 |
| One list call fails | `Err = <err>`, partial results from the other call shown |
| Both list calls fail | `Err = <combined>`, empty model list |

## Testing

Unit tests in `lmstudio_test.go`:
- Merge logic: downloaded + loaded → correct dedup, `IsLoaded` flag
- Handler with discovery failure → template renders error string

Integration tests (live server, `RUN_INTEGRATION=1 -tags=integration`): out of
scope for this change.
