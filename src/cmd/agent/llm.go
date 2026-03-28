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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	commonagent "github.com/refinement-systems/BorealValley/src/internal/common/agent"
)

type loopCallbacks struct {
	OnToolCall   func(name, args string) error
	OnToolResult func(name, result string) error
	OnAssistant  func(text string) error
}

type llmMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []llmToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type llmToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function llmToolFunction `json:"function"`
}

type llmToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type llmToolDef struct {
	Type     string             `json:"type"`
	Function llmToolFunctionDef `json:"function"`
}

type llmToolFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type llmChatRequest struct {
	Model       string       `json:"model"`
	Messages    []llmMessage `json:"messages"`
	Tools       []llmToolDef `json:"tools,omitempty"`
	Temperature float64      `json:"temperature"`
}

type llmChatChoice struct {
	Message      llmMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type llmChatResponse struct {
	Choices []llmChatChoice `json:"choices"`
}

func runLMStudioTicketLoop(ctx context.Context, lmstudioURL, model, workspace, ticketEnvelope string, maxIter int, cbs loopCallbacks) (string, error) {
	if strings.TrimSpace(lmstudioURL) == "" {
		return "", fmt.Errorf("lmstudio url required")
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("model required")
	}
	if strings.TrimSpace(workspace) == "" {
		return "", fmt.Errorf("workspace required")
	}
	if maxIter <= 0 {
		maxIter = 3
	}

	messages := []llmMessage{
		{
			Role: "system",
			Content: "You are a coding assistant operating one ticket at a time. " +
				"Use tools to inspect/edit workspace files before answering. " +
				"When tool calls are needed, return tool calls only.",
		},
		{
			Role:    "user",
			Content: ticketEnvelope,
		},
	}

	httpClient := lmStudioHTTPClient()
	lastContent := ""
	for i := 0; i < maxIter; i++ {
		body, err := json.Marshal(llmChatRequest{
			Model:       model,
			Messages:    messages,
			Tools:       agentTools(),
			Temperature: 0.2,
		})
		if err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(lmstudioURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil {
			return "", err
		}
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("lmstudio returned http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}

		var parsed llmChatResponse
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return "", fmt.Errorf("decode model response: %w", err)
		}
		if len(parsed.Choices) == 0 {
			return "", fmt.Errorf("lmstudio returned no choices")
		}

		choice := parsed.Choices[0]
		switch choice.FinishReason {
		case "tool_calls":
			if strings.TrimSpace(choice.Message.Content) != "" {
				lastContent = choice.Message.Content
			}
			messages = append(messages, choice.Message)
			for _, tc := range choice.Message.ToolCalls {
				if cbs.OnToolCall != nil {
					if err := cbs.OnToolCall(tc.Function.Name, tc.Function.Arguments); err != nil {
						return "", err
					}
				}
				toolResult := executeToolCall(ctx, workspace, tc)
				if cbs.OnToolResult != nil {
					if err := cbs.OnToolResult(tc.Function.Name, toolResult); err != nil {
						return "", err
					}
				}
				messages = append(messages, llmMessage{
					Role:       "tool",
					Content:    toolResult,
					ToolCallID: tc.ID,
				})
			}
		case "", "stop":
			answer := strings.TrimSpace(choice.Message.Content)
			if answer == "" {
				answer = strings.TrimSpace(lastContent)
			}
			if answer != "" && cbs.OnAssistant != nil {
				if err := cbs.OnAssistant(answer); err != nil {
					return "", err
				}
			}
			return answer, nil
		default:
			answer := strings.TrimSpace(choice.Message.Content)
			if answer == "" {
				answer = strings.TrimSpace(lastContent)
			}
			if answer != "" && cbs.OnAssistant != nil {
				if err := cbs.OnAssistant(answer); err != nil {
					return "", err
				}
			}
			return answer, nil
		}
	}

	if strings.TrimSpace(lastContent) != "" && cbs.OnAssistant != nil {
		if err := cbs.OnAssistant(lastContent); err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(lastContent), fmt.Errorf("stopped after %d round-trips (iteration limit)", maxIter)
}

func executeToolCall(ctx context.Context, workspace string, tc llmToolCall) string {
	args := map[string]string{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return `{"error":"failed to parse arguments"}`
	}

	var (
		result string
		err    error
	)
	switch tc.Function.Name {
	case "list_dir":
		result, err = commonagent.ListDir(workspace, args["path"])
	case "read_file":
		result, err = commonagent.ReadFile(workspace, args["path"])
	case "write_file":
		if err = commonagent.WriteFile(workspace, args["path"], args["content"]); err == nil {
			result = fmt.Sprintf(`{"ok":true,"path":%q}`, args["path"])
		}
	case "search_text":
		result, err = commonagent.SearchText(ctx, workspace, args["path"], args["query"])
	default:
		return fmt.Sprintf(`{"error":"unknown tool %q"}`, tc.Function.Name)
	}
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return result
}

func agentTools() []llmToolDef {
	return []llmToolDef{
		{
			Type: "function",
			Function: llmToolFunctionDef{
				Name:        "list_dir",
				Description: "List files/directories at path (sandboxed to workspace).",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
			},
		},
		{
			Type: "function",
			Function: llmToolFunctionDef{
				Name:        "read_file",
				Description: "Read a UTF-8 text file (sandboxed to workspace).",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
			},
		},
		{
			Type: "function",
			Function: llmToolFunctionDef{
				Name:        "write_file",
				Description: "Write content to a UTF-8 text file (sandboxed to workspace). Creates parent directories if needed. Overwrites existing files.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`),
			},
		},
		{
			Type: "function",
			Function: llmToolFunctionDef{
				Name:        "search_text",
				Description: "Search for a regex/string in files under a path (sandboxed to workspace).",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"query":{"type":"string"}},"required":["path","query"],"additionalProperties":false}`),
			},
		},
	}
}
