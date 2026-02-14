# Chat htmx + SSE with Conversation History — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the blocking POST-and-reload chat flow with htmx + SSE for live tool-call and answer updates, and add in-memory multi-turn conversation history scoped to each user session.

**Architecture:** A session-keyed `conversationStore` holds `[]chatMessage` (for the LLM) and `[]chatTurn` (for display). POST `/ctl/chat` creates a `chatJob`, starts a background agent loop with progress callbacks, and returns an SSE fragment. The SSE stream publishes `update` events (full re-render of the in-progress turn) and a final `done` event; the JS listener reloads the page on `done`, causing GET `/ctl/chat` to render the completed history.

**Tech Stack:** Go `net/http`, htmx 2.0.4, htmx-ext-sse 2.2.2, `github.com/hypernetix/lmstudio-go`, `github.com/alexedwards/scs/v2` (session token for store key).

---

### Task 1: Conversation store

**Files:**
- Create: `src/cmd/web/conversation.go`
- Create: `src/cmd/web/conversation_test.go`

**Step 1: Write the failing tests**

```go
// src/cmd/web/conversation_test.go
package main

import "testing"

func TestConversationStoreGetOrCreate(t *testing.T) {
	s := &conversationStore{convs: make(map[string]*conversation)}

	c1 := s.getOrCreate("tok1")
	c2 := s.getOrCreate("tok1")
	if c1 != c2 {
		t.Fatal("same token must return same conversation")
	}

	c3 := s.getOrCreate("tok2")
	if c1 == c3 {
		t.Fatal("different tokens must return different conversations")
	}
}

func TestConversationStoreClear(t *testing.T) {
	s := &conversationStore{convs: make(map[string]*conversation)}

	c := s.getOrCreate("tok1")
	c.mu.Lock()
	c.Turns = append(c.Turns, chatTurn{Prompt: "hello"})
	c.mu.Unlock()

	s.clear("tok1")

	c2 := s.getOrCreate("tok1")
	c2.mu.Lock()
	n := len(c2.Turns)
	c2.mu.Unlock()
	if n != 0 {
		t.Fatal("expected empty conversation after clear")
	}
}
```

**Step 2: Run to confirm failure**

```
go test ./src/cmd/web -run TestConversationStore
```
Expected: compile error (types not defined yet).

**Step 3: Implement**

```go
// src/cmd/web/conversation.go
package main

import "sync"

// chatTurn holds one user→assistant exchange for display.
type chatTurn struct {
	Prompt    string
	ToolCalls []toolCallTrace
	Answer    string
	Err       string
}

// conversation stores the full LLM message history and display turns for one
// session. All fields are protected by mu.
type conversation struct {
	mu       sync.Mutex
	Messages []chatMessage // system + all turns, sent to LM Studio each round
	Turns    []chatTurn    // display-only
}

type conversationStore struct {
	mu    sync.Mutex
	convs map[string]*conversation
}

var globalConversations = &conversationStore{
	convs: make(map[string]*conversation),
}

func (s *conversationStore) getOrCreate(token string) *conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.convs[token]
	if !ok {
		c = &conversation{}
		s.convs[token] = c
	}
	return c
}

func (s *conversationStore) clear(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.convs, token)
}
```

**Step 4: Run tests**

```
go test ./src/cmd/web -run TestConversationStore
```
Expected: PASS.

**Step 5: Build check**

```
just build-web
```

**Step 6: Commit**

```
git add src/cmd/web/conversation.go src/cmd/web/conversation_test.go
git commit -m "add in-memory conversation store"
```

---

### Task 2: Chat SSE job

**Files:**
- Create: `src/cmd/web/chat_sse.go`
- Create: `src/cmd/web/chat_sse_test.go`

**Step 1: Write the failing tests**

```go
// src/cmd/web/chat_sse_test.go
package main

import (
	"strings"
	"testing"
)

func TestChatJobUpdateHTML_Empty(t *testing.T) {
	j := &chatJob{subs: make(map[chan sseMsg]struct{})}
	got := chatJobUpdateHTML(j)
	if !strings.Contains(got, "Thinking") {
		t.Fatalf("expected 'Thinking' in initial HTML, got: %q", got)
	}
}

func TestChatJobUpdateHTML_WithTrace(t *testing.T) {
	j := &chatJob{subs: make(map[chan sseMsg]struct{})}
	j.traces = []toolCallTrace{
		{Name: "list_dir", Args: `{"path":"src"}`, Result: "file1.go"},
	}
	got := chatJobUpdateHTML(j)
	if !strings.Contains(got, "list_dir") {
		t.Fatalf("expected tool name in HTML, got: %q", got)
	}
	if !strings.Contains(got, "file1.go") {
		t.Fatalf("expected tool result in HTML, got: %q", got)
	}
}

func TestChatJobUpdateHTML_WithAnswer(t *testing.T) {
	j := &chatJob{subs: make(map[chan sseMsg]struct{})}
	j.answer = "The answer is 42."
	got := chatJobUpdateHTML(j)
	if !strings.Contains(got, "The answer is 42.") {
		t.Fatalf("expected answer in HTML, got: %q", got)
	}
}

func TestChatJobUpdateHTML_WithError(t *testing.T) {
	j := &chatJob{subs: make(map[chan sseMsg]struct{})}
	j.jobErr = "something went wrong"
	got := chatJobUpdateHTML(j)
	if !strings.Contains(got, "something went wrong") {
		t.Fatalf("expected error in HTML, got: %q", got)
	}
}
```

**Step 2: Run to confirm failure**

```
go test ./src/cmd/web -run TestChatJob
```
Expected: compile error.

**Step 3: Implement**

```go
// src/cmd/web/chat_sse.go
package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"
)

type chatJob struct {
	ID string

	mu            sync.Mutex
	currentStatus string
	traces        []toolCallTrace
	answer        string
	jobErr        string
	done          bool

	subsMu sync.Mutex
	subs   map[chan sseMsg]struct{}
}

func newChatJob() *chatJob {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return &chatJob{
		ID:            hex.EncodeToString(b),
		currentStatus: "Thinking\u2026",
		subs:          make(map[chan sseMsg]struct{}),
	}
}

// setStatus updates the status line and publishes an update event.
func (j *chatJob) setStatus(s string) {
	j.mu.Lock()
	j.currentStatus = s
	j.mu.Unlock()
	j.publish(sseMsg{event: "update", data: chatJobUpdateHTML(j)})
}

// addTrace appends a completed tool call trace and publishes an update.
func (j *chatJob) addTrace(t toolCallTrace) {
	j.mu.Lock()
	j.traces = append(j.traces, t)
	j.currentStatus = "Thinking\u2026"
	j.mu.Unlock()
	j.publish(sseMsg{event: "update", data: chatJobUpdateHTML(j)})
}

// setAnswer records the final answer and publishes an update.
func (j *chatJob) setAnswer(text string) {
	j.mu.Lock()
	j.answer = text
	j.currentStatus = ""
	j.mu.Unlock()
	j.publish(sseMsg{event: "update", data: chatJobUpdateHTML(j)})
}

// complete marks the job done. If err != nil the error is shown.
// Always publishes a final update then a done event.
func (j *chatJob) complete(err error) {
	j.mu.Lock()
	j.done = true
	j.currentStatus = ""
	if err != nil && j.answer == "" {
		j.jobErr = err.Error()
	}
	j.mu.Unlock()
	j.publish(sseMsg{event: "update", data: chatJobUpdateHTML(j)})
	j.publish(sseMsg{event: "done", data: ""})
}

func (j *chatJob) publish(msg sseMsg) {
	j.subsMu.Lock()
	defer j.subsMu.Unlock()
	for ch := range j.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (j *chatJob) subscribe() (chan sseMsg, func()) {
	ch := make(chan sseMsg, 16)
	j.subsMu.Lock()
	j.subs[ch] = struct{}{}
	j.subsMu.Unlock()
	return ch, func() {
		j.subsMu.Lock()
		delete(j.subs, ch)
		j.subsMu.Unlock()
		close(ch)
	}
}

type chatJobStore struct {
	mu   sync.Mutex
	jobs map[string]*chatJob
}

var globalChatJobs = &chatJobStore{jobs: make(map[string]*chatJob)}

func (s *chatJobStore) new() *chatJob {
	j := newChatJob()
	s.mu.Lock()
	s.jobs[j.ID] = j
	s.mu.Unlock()
	return j
}

func (s *chatJobStore) get(id string) (*chatJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

// modelJobEventsHandler serves GET /ctl/chat/events/{id} as an SSE stream.
func (app *application) chatJobEventsHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := globalChatJobs.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch, unsubscribe := j.subscribe()
	defer unsubscribe()

	// Send initial snapshot.
	j.mu.Lock()
	done := j.done
	j.mu.Unlock()
	writeSSEEvent(w, "update", chatJobUpdateHTML(j))
	if done {
		writeSSEEvent(w, "done", "")
	}
	flusher.Flush()

	if done {
		return
	}

	ctx := r.Context()
	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepAlive.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			writeSSEEvent(w, msg.event, msg.data)
			flusher.Flush()
			if msg.event == "done" {
				return
			}
		}
	}
}

// chatJobUpdateHTML renders the current in-progress turn as an HTML fragment.
func chatJobUpdateHTML(j *chatJob) string {
	j.mu.Lock()
	status := j.currentStatus
	traces := make([]toolCallTrace, len(j.traces))
	copy(traces, j.traces)
	answer := j.answer
	jobErr := j.jobErr
	j.mu.Unlock()

	var sb strings.Builder
	for _, t := range traces {
		fmt.Fprintf(&sb,
			`<details><summary><code>%s</code> <small>%s</small></summary><pre>%s</pre></details>`,
			template.HTMLEscapeString(t.Name),
			template.HTMLEscapeString(t.Args),
			template.HTMLEscapeString(t.Result),
		)
	}
	if jobErr != "" {
		fmt.Fprintf(&sb, `<p style="color:red">%s</p>`,
			template.HTMLEscapeString(jobErr))
	} else if answer != "" {
		fmt.Fprintf(&sb, `<pre style="white-space:pre-wrap">%s</pre>`,
			template.HTMLEscapeString(answer))
	} else if status != "" {
		fmt.Fprintf(&sb, `<p><em>%s</em></p>`,
			template.HTMLEscapeString(status))
	}
	return sb.String()
}
```

**Step 4: Run tests**

```
go test ./src/cmd/web -run TestChatJob
```
Expected: PASS.

**Step 5: Build check**

```
just build-web
```

**Step 6: Commit**

```
git add src/cmd/web/chat_sse.go src/cmd/web/chat_sse_test.go
git commit -m "add chat SSE job and events handler"
```

---

### Task 3: Update runAgentLoop to accept messages + callbacks

**Files:**
- Modify: `src/cmd/web/chat.go`

The signature changes from `(baseURL, model, prompt string) (string, []toolCallTrace, error)` to accept the full messages slice and callbacks, and return the final updated messages.

**Step 1: Read existing tests** to understand what needs updating:

```
go test ./src/cmd/web -v -run TestRunAgent 2>&1 | head -40
```

**Step 2: Update `chat.go`**

Add `agentCallbacks` struct and change `runAgentLoop` signature. The prompt-to-messages construction moves to the caller (`chatCtlPost`). Key changes inside the loop:

- After `tool_calls` finish reason: for each tool call, call `cbs.onToolCall(name, args)`, execute, call `cbs.onToolResult(name, result)`.
- After `stop` finish reason: call `cbs.onAnswer(content)`, append final assistant message to messages, return updated messages.
- Nil-check every callback before calling.
- Return `(string, []chatMessage, []toolCallTrace, error)`.

Replace the existing `runAgentLoop` function body in `src/cmd/web/chat.go`:

```go
type agentCallbacks struct {
	onToolCall   func(name, args string)
	onToolResult func(name, result string)
	onAnswer     func(text string)
}

// runAgentLoop sends messages to LM Studio's OpenAI-compatible chat completions
// endpoint, executing tool calls until the model stops or the iteration limit is
// hit. It returns the final answer, the complete updated message history
// (including all tool and assistant messages appended during the loop), the
// tool call traces, and any error.
func (app *application) runAgentLoop(
	baseURL, model string,
	messages []chatMessage,
	cbs agentCallbacks,
) (answer string, finalMessages []chatMessage, traces []toolCallTrace, err error) {
	const maxIter = 3

	tools := agentTools()
	httpClient := &http.Client{}
	var lastContent string

	for i := 0; i < maxIter; i++ {
		slog.Debug("agent loop iteration", "iter", i, "model", model, "messages", len(messages))

		reqBody, err := json.Marshal(chatRequest{
			Model:       model,
			Messages:    messages,
			Tools:       tools,
			Temperature: 0.2,
		})
		if err != nil {
			return "", messages, traces, fmt.Errorf("failed to marshal request: %w", err)
		}
		slog.Debug("sending chat request", "iter", i, "body", string(reqBody))

		resp, err := httpClient.Post(baseURL+"/v1/chat/completions", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			return "", messages, traces, fmt.Errorf("chat request failed: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", messages, traces, fmt.Errorf("failed to read response: %w", err)
		}
		slog.Debug("received chat response", "iter", i, "status", resp.StatusCode, "body", string(body))

		if resp.StatusCode >= 400 {
			return "", messages, traces, fmt.Errorf("model returned HTTP %d: %s", resp.StatusCode, body)
		}

		var cr chatResponse
		if err := json.Unmarshal(body, &cr); err != nil {
			return "", messages, traces, fmt.Errorf("failed to decode response: %w", err)
		}
		if len(cr.Choices) == 0 {
			return "", messages, traces, fmt.Errorf("no choices in response")
		}

		choice := cr.Choices[0]
		slog.Info("model response", "iter", i, "finish_reason", choice.FinishReason, "tool_calls", len(choice.Message.ToolCalls))

		switch choice.FinishReason {
		case "stop", "":
			if cbs.onAnswer != nil {
				cbs.onAnswer(choice.Message.Content)
			}
			messages = append(messages, choice.Message)
			slog.Info("agent loop done", "iter", i, "answer_len", len(choice.Message.Content))
			return choice.Message.Content, messages, traces, nil

		case "tool_calls":
			if choice.Message.Content != "" {
				lastContent = choice.Message.Content
			}
			messages = append(messages, choice.Message)
			for _, tc := range choice.Message.ToolCalls {
				slog.Info("executing tool", "name", tc.Function.Name, "args", tc.Function.Arguments)
				if cbs.onToolCall != nil {
					cbs.onToolCall(tc.Function.Name, tc.Function.Arguments)
				}
				toolResult := app.executeTool(tc)
				slog.Debug("tool result", "name", tc.Function.Name, "result", toolResult)
				if cbs.onToolResult != nil {
					cbs.onToolResult(tc.Function.Name, toolResult)
				}
				traces = append(traces, toolCallTrace{
					Name:   tc.Function.Name,
					Args:   tc.Function.Arguments,
					Result: toolResult,
				})
				messages = append(messages, chatMessage{
					Role:       "tool",
					Content:    toolResult,
					ToolCallID: tc.ID,
				})
			}

		default:
			slog.Info("unexpected finish_reason", "finish_reason", choice.FinishReason)
			messages = append(messages, choice.Message)
			return choice.Message.Content, messages, traces, nil
		}
	}

	slog.Info("agent loop hit iteration limit", "maxIter", maxIter, "partial_content_len", len(lastContent))
	if lastContent != "" {
		messages = append(messages, chatMessage{Role: "assistant", Content: lastContent})
	}
	return lastContent, messages, traces, fmt.Errorf("stopped after %d round-trips (iteration limit)", maxIter)
}
```

Remove the old `runAgentLoop` body entirely and replace with the above (keep the function, replace the body and signature).

**Step 3: Run tests**

```
go test ./src/cmd/web
```

Expected: compile errors in `chatCtlPost` because caller signature changed. Fix in next task.

---

### Task 4: Update chatCtlPost, chatCtlGet, add reset handler

**Files:**
- Modify: `src/cmd/web/chat.go`

**Step 1: Update `chatCtlData` struct**

Remove `Prompt`, `Result`, `ToolCalls` fields (no longer needed for GET render; the POST now returns a fragment). Add `Turns`:

```go
type chatCtlData struct {
	Models []modelRow
	Turns  []chatTurn
	Err    string
}
```

**Step 2: Add `chatJobTmpl`**

```go
// chatJobTmpl renders the SSE-connected fragment returned when a chat job starts.
// Template data is the job ID string.
var chatJobTmpl = template.Must(template.New("chat-job").Parse(`<div hx-ext="sse" sse-connect="/ctl/chat/events/{{.}}" sse-close="done">
  <div sse-swap="update"><p><em>Thinking&#8230;</em></p></div>
  <div sse-swap="done"></div>
</div>
`))
```

**Step 3: Update `chatCtlGet`**

Load conversation history from store and pass turns to template:

```go
func (app *application) chatCtlGet(w http.ResponseWriter, r *http.Request) {
	data := chatCtlData{}

	addr, err := lmstudioDiscover()
	if err != nil {
		data.Err = "LM Studio not found: " + err.Error()
		renderChatCtl(w, data)
		return
	}

	log := lmstudio.NewLogger(lmstudio.LogLevelError)
	client := lmstudio.NewLMStudioClient(addr, log)
	defer client.Close()

	loaded, err := client.ListAllLoadedModels()
	if err != nil {
		data.Err = "failed to list models: " + err.Error()
		renderChatCtl(w, data)
		return
	}

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
		data.Models = append(data.Models, modelRow{Name: name, Key: id})
	}

	token := app.sessionManager.Token(r.Context())
	conv := globalConversations.getOrCreate(token)
	conv.mu.Lock()
	data.Turns = make([]chatTurn, len(conv.Turns))
	copy(data.Turns, conv.Turns)
	conv.mu.Unlock()

	renderChatCtl(w, data)
}
```

**Step 4: Replace `chatCtlPost`**

```go
func (app *application) chatCtlPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	model := r.PostFormValue("model")
	prompt := r.PostFormValue("prompt")

	if model == "" || prompt == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<p style="color:red">model and prompt are required</p>`)
		return
	}

	addr, err := lmstudioDiscover()
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<p style="color:red">LM Studio not found: %s</p>`,
			template.HTMLEscapeString(err.Error()))
		return
	}

	token := app.sessionManager.Token(r.Context())
	conv := globalConversations.getOrCreate(token)

	// Build input messages: initialise system message on first turn.
	conv.mu.Lock()
	if len(conv.Messages) == 0 {
		conv.Messages = []chatMessage{{
			Role: "system",
			Content: "You are a coding assistant with access to tools that let you inspect the workspace. " +
				"Always use tools to read files or search the codebase before answering questions about the code. " +
				"When you call a tool, do not include extra prose — only the tool call. " +
				"After receiving tool results, use them to form your final answer.",
		}}
	}
	conv.Messages = append(conv.Messages, chatMessage{Role: "user", Content: prompt})
	inputMessages := make([]chatMessage, len(conv.Messages))
	copy(inputMessages, conv.Messages)
	conv.mu.Unlock()

	j := globalChatJobs.new()

	go func() {
		answer, finalMessages, traces, loopErr := app.runAgentLoop(addr, model, inputMessages, agentCallbacks{
			onToolCall: func(name, args string) {
				j.setStatus("Calling " + name + "\u2026")
			},
			onToolResult: func(name, result string) {
				// addTrace is called after executeTool in runAgentLoop,
				// but we need the full trace; use a closure variable.
			},
			onAnswer: func(text string) {
				j.setAnswer(text)
			},
		})
		// Publish completed traces (answer already set via onAnswer callback).
		// We rebuild from finalMessages here but it's simpler to accumulate
		// in the job via a different callback. See note below.
		_ = finalMessages // used to persist history only

		turn := chatTurn{Prompt: prompt, ToolCalls: traces}
		if loopErr != nil && answer == "" {
			turn.Err = loopErr.Error()
		} else {
			turn.Answer = answer
		}

		conv.mu.Lock()
		conv.Messages = finalMessages
		conv.Turns = append(conv.Turns, turn)
		conv.mu.Unlock()

		j.complete(loopErr)
	}()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = chatJobTmpl.Execute(w, j.ID)
}
```

**Important:** The `onToolResult` callback above is a stub. To show tool call traces live in the SSE stream, pass a proper `onToolResult` that calls `j.addTrace`. Since `runAgentLoop` calls `onToolCall` before execution and `onToolResult` after, we can accumulate a partial trace:

Replace the callbacks in `chatCtlPost`:

```go
var pendingName, pendingArgs string

agentCallbacks{
	onToolCall: func(name, args string) {
		pendingName = args  // BUG: wrong field
		j.setStatus("Calling " + name + "\u2026")
	},
```

Actually, do it cleanly with a small closure struct:

```go
var (
	pendingName string
	pendingArgs string
)
cbs := agentCallbacks{
	onToolCall: func(name, args string) {
		pendingName = name
		pendingArgs = args
		j.setStatus("Calling " + name + "\u2026")
	},
	onToolResult: func(_, result string) {
		j.addTrace(toolCallTrace{
			Name:   pendingName,
			Args:   pendingArgs,
			Result: result,
		})
	},
	onAnswer: func(text string) {
		j.setAnswer(text)
	},
}
```

Use `cbs` when calling `app.runAgentLoop`. The full corrected `chatCtlPost` goroutine:

```go
go func() {
	var (
		pendingName string
		pendingArgs string
	)
	cbs := agentCallbacks{
		onToolCall: func(name, args string) {
			pendingName = name
			pendingArgs = args
			j.setStatus("Calling " + name + "\u2026")
		},
		onToolResult: func(_, result string) {
			j.addTrace(toolCallTrace{
				Name:   pendingName,
				Args:   pendingArgs,
				Result: result,
			})
		},
		onAnswer: func(text string) {
			j.setAnswer(text)
		},
	}

	answer, finalMessages, traces, loopErr := app.runAgentLoop(addr, model, inputMessages, cbs)

	turn := chatTurn{Prompt: prompt, ToolCalls: traces}
	if loopErr != nil && answer == "" {
		turn.Err = loopErr.Error()
	} else {
		turn.Answer = answer
	}

	conv.mu.Lock()
	conv.Messages = finalMessages
	conv.Turns = append(conv.Turns, turn)
	conv.mu.Unlock()

	j.complete(loopErr)
}()
```

**Step 5: Add `chatCtlReset`**

```go
func (app *application) chatCtlReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := app.sessionManager.Token(r.Context())
	globalConversations.clear(token)
	http.Redirect(w, r, "/ctl/chat", http.StatusSeeOther)
}
```

**Step 6: Run tests**

```
go test ./src/cmd/web
```
Expected: PASS (all existing tests pass, new logic not yet covered by tests).

**Step 7: Build check**

```
just build-web
```

**Step 8: Commit**

```
git add src/cmd/web/chat.go
git commit -m "update chat: SSE-driven agent loop with conversation history"
```

---

### Task 5: Register new routes

**Files:**
- Modify: `src/cmd/web/main.go`

**Step 1: Add routes**

After the existing `/ctl/chat` line, add:

```go
mux.HandleFunc("GET /ctl/chat/events/{id}", app.requireAuth(app.chatJobEventsHandler))
mux.HandleFunc("POST /ctl/chat/reset", app.requireAuth(app.chatCtlReset))
```

**Step 2: Build + test**

```
just build-web && go test ./src/cmd/web
```
Expected: PASS.

**Step 3: Commit**

```
git add src/cmd/web/main.go
git commit -m "register chat SSE and reset routes"
```

---

### Task 6: Update ctl-chat.html template

**Files:**
- Modify: `src/internal/assets/html/ctl-chat.html`

**Step 1: Rewrite the template**

```html
<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
    <script src="https://unpkg.com/htmx-ext-sse@2.2.2/sse.js"></script>
  </head>
  <body>
    <h1>Chat</h1>
    {{if .Err}}<p style="color:red">{{.Err}}</p>{{end}}

    <div id="history">
      {{range .Turns}}
      <div style="margin-bottom:1em;border-bottom:1px solid #ccc;padding-bottom:0.5em">
        <p><strong>You:</strong> {{.Prompt}}</p>
        {{range .ToolCalls}}
        <details>
          <summary><code>{{.Name}}</code> &mdash; <small>{{.Args}}</small></summary>
          <pre>{{.Result}}</pre>
        </details>
        {{end}}
        {{if .Err}}
        <p style="color:red">{{.Err}}</p>
        {{else}}
        <pre style="white-space:pre-wrap">{{.Answer}}</pre>
        {{end}}
      </div>
      {{end}}
    </div>

    <div id="current-response"></div>

    <form hx-post="/ctl/chat" hx-target="#current-response" hx-swap="innerHTML">
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
      <button type="submit">Send</button>
    </form>

    <form method="POST" action="/ctl/chat/reset" style="margin-top:0.5em">
      <button type="submit">New conversation</button>
    </form>

    <p><a href="/">Home</a> | <a href="/ctl/model">Models</a></p>

    <script>
      htmx.on('htmx:sseMessage', function(e) {
        if (e.detail.type === 'done') {
          window.location.reload();
        }
      });
    </script>
  </body>
</html>
```

**Step 2: Build + test**

```
just build-web && go test ./src/cmd/web
```
Expected: PASS.

**Step 3: Commit**

```
git add src/internal/assets/html/ctl-chat.html
git commit -m "update chat template: htmx SSE, history, new conversation button"
```

---

### Task 7: Smoke test end-to-end

**Step 1: Start server**

```
go run ./src/cmd/web -db app.db -addr :4000 -env dev
```

**Step 2: Verify**

1. Open `http://localhost:4000/ctl/chat`
2. Select a loaded model, type a prompt, click Send
3. Confirm `#current-response` shows "Thinking…" immediately, then tool call details as they run, then the final answer
4. Confirm on completion the page reloads and the turn appears in `#history`
5. Send a follow-up prompt — confirm the model receives the full prior context
6. Click "New conversation" — confirm history clears

**Step 3: Final test run**

```
go test ./...
```
Expected: all packages pass.
