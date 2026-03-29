package main

import (
	"strings"
	"testing"
)

func TestSystemPromptDefault(t *testing.T) {
	got := buildSystemPrompt(CollabModeDefault)
	if !strings.Contains(got, "coding assistant") {
		t.Errorf("default prompt missing expected content, got: %q", got)
	}
}

func TestSystemPromptPlanContainsBase(t *testing.T) {
	got := buildSystemPrompt(CollabModePlan)
	if !strings.Contains(got, baseSystemPrompt) {
		t.Errorf("plan prompt missing base prompt, got: %q", got)
	}
}

func TestSystemPromptPlanContainsAddendum(t *testing.T) {
	got := buildSystemPrompt(CollabModePlan)
	if !strings.Contains(got, "<proposed_plan>") {
		t.Errorf("plan prompt missing <proposed_plan> tag, got: %q", got)
	}
}

func TestSystemPromptPlanContainsPlanMode(t *testing.T) {
	got := buildSystemPrompt(CollabModePlan)
	if !strings.Contains(got, "plan mode") {
		t.Errorf("plan prompt missing 'plan mode' text, got: %q", got)
	}
}
