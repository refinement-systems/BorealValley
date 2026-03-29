package main

const baseSystemPrompt = "You are a coding assistant operating one ticket at a time. " +
	"Use tools to inspect/edit workspace files before answering. " +
	"When tool calls are needed, return tool calls only."

const planModeAddendum = "You are in plan mode. Do NOT modify any files. " +
	"Use read-only tools (list_dir, read_file, search_text) to understand the codebase. " +
	"Produce your plan wrapped in <proposed_plan> and </proposed_plan> XML tags. " +
	"The plan should describe what files to change, what the changes should be, and why. " +
	"Do not use write_file. If you attempt to write, your call will be rejected."

func buildSystemPrompt(mode CollaborationMode) string {
	if mode == CollabModePlan {
		return baseSystemPrompt + "\n\n" + planModeAddendum
	}
	return baseSystemPrompt
}
