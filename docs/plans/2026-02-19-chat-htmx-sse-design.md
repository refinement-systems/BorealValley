# Chat htmx + SSE with Conversation History

**Date:** 2026-02-19
**Status:** Approved

## Goal

Replace the blocking POST-and-reload chat flow with htmx + Server-Sent Events so
tool calls and the final answer appear live, and add in-memory multi-turn
conversation history scoped to each user session.

## Architecture

### Conversation store

```go
type chatTurn struct {
    Prompt    string
    ToolCalls []toolCallTrace
    Answer    string
    Err       string
}

type conversation struct {
    Messages []chatMessage // full LLM message list (system + all turns)
    Turns    []chatTurn    // display-only list
}

// global, keyed by SCS session token
var globalConversations = &conversationStore{...}
```

Session token from `app.sessionManager.Token(r.Context())` is the map key.
History is in-memory only; lost on server restart.

### Chat job (SSE producer)

A `chatJob` (similar to `modelJob` in model_sse.go) tracks in-progress state:

- `currentStatus string` — e.g. "Thinking…", "Calling list_dir…"
- `traces []toolCallTrace` — completed tool calls so far
- `answer string` — final answer (empty until done)
- `err string` — error if loop fails

It publishes a single SSE event named `update` carrying a full re-render of
the in-progress turn HTML each time state changes. A final `done` event closes
the connection.

### Agent loop changes

`runAgentLoop` gains progress callbacks via a new `agentCallbacks` struct:

```go
type agentCallbacks struct {
    onToolCall   func(name, args string)
    onToolResult func(name, result string)
    onAnswer     func(text string)
}
```

These are called at the appropriate points in the loop and drive SSE publishes.

### Routes

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/ctl/chat` | Render full page with history + form |
| POST | `/ctl/chat` | Start chat job; return SSE fragment |
| POST | `/ctl/chat/reset` | Clear session conversation; redirect to GET |
| GET | `/ctl/chat/events/{id}` | SSE stream for a chat job |

### Page layout

```
<div id="history">   ← all past turns from session, rendered on GET
  [turn: prompt / tool calls / answer]
  ...
</div>
<div id="current-response">   ← htmx swap target; receives SSE fragment
</div>
<form hx-post="/ctl/chat" hx-target="#current-response" hx-swap="innerHTML">
  model select | prompt textarea | Send button
</form>
<form action="/ctl/chat/reset" method="POST">New conversation</form>
```

### SSE event flow

1. POST `/ctl/chat` → creates job, appends user message to conversation,
   starts goroutine running agent loop, returns SSE fragment into
   `#current-response`.
2. Job publishes `update` events as: loop start → each tool call → each
   tool result → final answer.
3. Job publishes `done` event; SSE connection closes (`sse-close="done"`).
4. JS listener on `htmx:sseMessage` with `type === "done"` calls
   `window.location.reload()`.
5. Reloaded page renders full conversation history including the new turn.

### Error handling

If the agent loop errors, the error is stored in the turn and the `done` event
is still sent so the page reloads and shows the error in the history.

### "New conversation" button

`POST /ctl/chat/reset` deletes the session's conversation entry and redirects
to `GET /ctl/chat`.
