// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// blockWriteFile is a ToolApprovalFunc that blocks write_file and allows everything else.
func blockWriteFile(toolName string, _ map[string]string) (bool, string) {
	if toolName == "write_file" {
		return false, "write_file is not allowed in plan mode"
	}
	return true, ""
}

// toolCallsResponse returns an LM Studio response that requests a single tool call.
func toolCallsResponse(toolName, arguments string) map[string]any {
	return map[string]any{
		"choices": []map[string]any{{
			"message": map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{{
					"id":   "call-1",
					"type": "function",
					"function": map[string]any{
						"name":      toolName,
						"arguments": arguments,
					},
				}},
			},
			"finish_reason": "tool_calls",
		}},
	}
}

// stopResponse returns a simple stop response.
func stopResponse(content string) map[string]any {
	return map[string]any{
		"choices": []map[string]any{{
			"message":       map[string]any{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
	}
}

// twoRoundHandler returns a fakeLMStudioServer handler that returns tool_calls on round 1
// and calls onRound2 to get the response on round 2. onRound2 may be nil for a plain stop.
func twoRoundHandler(round1 map[string]any, onRound2 func(r *http.Request) map[string]any) func(r *http.Request) (int, any) {
	callCount := 0
	return func(r *http.Request) (int, any) {
		callCount++
		if callCount == 1 {
			return http.StatusOK, round1
		}
		if onRound2 != nil {
			return http.StatusOK, onRound2(r)
		}
		return http.StatusOK, stopResponse("done")
	}
}

// TestApprovalNilAllowsAll verifies that nil ApproveToolCall does not block any tool.
func TestApprovalNilAllowsAll(t *testing.T) {
	workspace := t.TempDir()
	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{
		handler: twoRoundHandler(toolCallsResponse("read_file", `{"path":"missing.txt"}`), nil),
	}.serveHTTP))

	var toolResults []string
	_, err := runLMStudioTicketLoop(context.Background(), lm.URL, "m", workspace, "ticket", 3, CollabModeDefault, loopCallbacks{
		OnToolResult: func(name, result string) error {
			toolResults = append(toolResults, result)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(toolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(toolResults))
	}
	if strings.Contains(toolResults[0], "tool_blocked") {
		t.Fatalf("nil gate should not block: %s", toolResults[0])
	}
}

// TestApprovalBlocksWriteFile verifies that a blocking gate prevents write_file from executing
// and the model receives {"error":"tool_blocked",...} as the tool result.
func TestApprovalBlocksWriteFile(t *testing.T) {
	workspace := t.TempDir()

	var capturedMessages []llmMessage
	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{
		handler: twoRoundHandler(
			toolCallsResponse("write_file", `{"path":"foo.txt","content":"bar"}`),
			func(r *http.Request) map[string]any {
				body, _ := io.ReadAll(r.Body)
				var req llmChatRequest
				_ = json.Unmarshal(body, &req)
				capturedMessages = req.Messages
				return stopResponse("done")
			},
		),
	}.serveHTTP))

	var toolResults []string
	_, err := runLMStudioTicketLoop(context.Background(), lm.URL, "m", workspace, "ticket", 3, CollabModeDefault, loopCallbacks{
		ApproveToolCall: blockWriteFile,
		OnToolResult: func(name, result string) error {
			toolResults = append(toolResults, result)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(toolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(toolResults))
	}
	if !strings.Contains(toolResults[0], "tool_blocked") {
		t.Fatalf("expected tool_blocked result, got: %s", toolResults[0])
	}
	// Verify the model received the blocked message in the next round.
	found := false
	for _, m := range capturedMessages {
		if m.Role == "tool" && strings.Contains(m.Content, "tool_blocked") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tool_blocked in round-2 messages, got: %v", capturedMessages)
	}
}

// TestApprovalAllowsReadFile verifies that an approval gate that only blocks write_file
// still allows read_file to execute normally.
func TestApprovalAllowsReadFile(t *testing.T) {
	workspace := t.TempDir()
	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{
		handler: twoRoundHandler(toolCallsResponse("read_file", `{"path":"missing.txt"}`), nil),
	}.serveHTTP))

	var toolResults []string
	_, err := runLMStudioTicketLoop(context.Background(), lm.URL, "m", workspace, "ticket", 3, CollabModeDefault, loopCallbacks{
		ApproveToolCall: blockWriteFile,
		OnToolResult: func(name, result string) error {
			toolResults = append(toolResults, result)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(toolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(toolResults))
	}
	if strings.Contains(toolResults[0], "tool_blocked") {
		t.Fatalf("read_file should not be blocked: %s", toolResults[0])
	}
}

// TestApprovalOnToolCallFiresWhenBlocked verifies that OnToolCall fires even when the tool is blocked.
func TestApprovalOnToolCallFiresWhenBlocked(t *testing.T) {
	workspace := t.TempDir()
	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{
		handler: twoRoundHandler(toolCallsResponse("write_file", `{"path":"foo.txt","content":"bar"}`), nil),
	}.serveHTTP))

	var toolCallNames []string
	_, err := runLMStudioTicketLoop(context.Background(), lm.URL, "m", workspace, "ticket", 3, CollabModeDefault, loopCallbacks{
		ApproveToolCall: blockWriteFile,
		OnToolCall: func(name, args string) error {
			toolCallNames = append(toolCallNames, name)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(toolCallNames) != 1 || toolCallNames[0] != "write_file" {
		t.Fatalf("expected OnToolCall to fire for write_file, got: %v", toolCallNames)
	}
}

// TestApprovalOnToolResultFiresWithBlockedContent verifies that OnToolResult fires with
// the blocked JSON when a tool is denied.
func TestApprovalOnToolResultFiresWithBlockedContent(t *testing.T) {
	workspace := t.TempDir()
	lm := newHTTPServer(t, http.HandlerFunc(fakeLMStudioServer{
		handler: twoRoundHandler(toolCallsResponse("write_file", `{"path":"foo.txt","content":"bar"}`), nil),
	}.serveHTTP))

	var resultNames []string
	var resultContents []string
	_, err := runLMStudioTicketLoop(context.Background(), lm.URL, "m", workspace, "ticket", 3, CollabModeDefault, loopCallbacks{
		ApproveToolCall: blockWriteFile,
		OnToolResult: func(name, result string) error {
			resultNames = append(resultNames, name)
			resultContents = append(resultContents, result)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resultNames) != 1 || resultNames[0] != "write_file" {
		t.Fatalf("expected OnToolResult for write_file, got: %v", resultNames)
	}
	if !strings.Contains(resultContents[0], "tool_blocked") {
		t.Fatalf("expected tool_blocked in OnToolResult content, got: %s", resultContents[0])
	}
}
