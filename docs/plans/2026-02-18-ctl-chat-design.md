# Design: /ctl/chat — LM Studio Agent Chat

Date: 2026-02-18

## Overview

Add a `/ctl/chat` endpoint to the BorealValley web server. It renders a form
where the user selects a loaded LM Studio model and enters a prompt. On submit,
the server runs a server-side agent loop: it calls the model, executes any tool
calls it makes, feeds results back, and repeats until the model stops. The final
answer is rendered on the page.

## New files

- `src/cmd/web/chat.go` — handler, agent loop, OpenAI-compatible HTTP client types
- `src/cmd/web/tools.go` — sandbox-safe tool implementations (list_dir, read_file, search_text)
- `src/internal/assets/html/ctl-chat.html` — page template

## Changed files

- `src/internal/assets/assets.go` — embed ctl-chat.html as HtmlCtlChat
- `src/cmd/web/main.go` — register route GET/POST /ctl/chat; store repoRoot on application

## Data flow

### GET /ctl/chat

1. Call lmstudioDiscover() (reuses existing package-level var from lmstudio.go)
2. Call client.ListAllLoadedModels(), filter to type == "llm"
3. Render form: model dropdown (loaded LLM identifiers) + prompt textarea + submit

### POST /ctl/chat

1. Parse form values: prompt, model
2. Call lmstudioDiscover() to get base URL
3. Build initial messages: [{role:"system", content:"..."}, {role:"user", content:prompt}]
4. Define 3 tools in the request: list_dir, read_file, search_text
5. Agent loop (max 10 iterations):
   a. POST to {baseURL}/v1/chat/completions with {model, messages, tools, temperature:0.2}
   b. If finish_reason == "stop" → use choices[0].message.content as final answer, break
   c. If finish_reason == "tool_calls" → execute each named tool, append:
      - assistant message (with tool_calls)
      - one tool message per call (tool_call_id, content=JSON result)
6. Render: tool-call trace + final answer (or error)

## OpenAI-compatible types (chat.go)

Minimal structs, no external dependency:

```go
type chatMessage struct {
    Role       string     `json:"role"`
    Content    string     `json:"content,omitempty"`
    ToolCalls  []toolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
}
type toolCall struct {
    ID       string       `json:"id"`
    Type     string       `json:"type"`
    Function toolCallFunc `json:"function"`
}
type toolCallFunc struct {
    Name      string `json:"name"`
    Arguments string `json:"arguments"`
}
```

Full request/response wrapper structs for chat completions.

## Tools (tools.go)

Sandbox root = application.repoRoot (set via os.Getwd() at startup).

- sandboxed(root, path) helper: filepath.Join + filepath.Clean, error if outside root
- list_dir(path) → os.ReadDir → JSON [{name, isDir, size}]
- read_file(path) → os.ReadFile, capped at 32KB
- search_text(path, query) → filepath.WalkDir + regex match per line, capped at 100 matches → [{file, line, text}]

## Template (ctl-chat.html)

- Form: model dropdown + prompt textarea + submit button
- If result available:
  - Tool trace: each tool call in <details><summary>toolName(args)</summary>result</details>
  - Final answer block
- Error shown in red if present

## Sandbox security

All file paths for tools are validated: filepath.Join(repoRoot, userPath) must have
repoRoot as a prefix after filepath.Clean. Paths escaping the sandbox return an error
string to the model (not a server error).

## Agent loop safety

Max 10 iterations to prevent runaway loops. If the limit is hit, the last partial
response (if any) is returned with a note about the iteration limit.
