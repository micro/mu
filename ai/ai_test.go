package ai

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_CustomSystemNoRag(t *testing.T) {
	p := &Prompt{
		System:   "Custom system prompt.",
		Question: "What is Bitcoin?",
	}
	got, err := BuildSystemPrompt(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Custom system prompt." {
		t.Errorf("expected custom system prompt unchanged, got %q", got)
	}
}

func TestBuildSystemPrompt_CustomSystemWithRag(t *testing.T) {
	p := &Prompt{
		System:   "Answer using ONLY the tool results below.",
		Rag:      []string{"### markets\nLive crypto market prices:\n- BTC: $68000.00"},
		Question: "What is the Bitcoin price?",
	}
	got, err := BuildSystemPrompt(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "Answer using ONLY the tool results below.") {
		t.Errorf("expected custom system prompt in output, got %q", got)
	}
	if !strings.Contains(got, "BTC: $68000.00") {
		t.Errorf("expected RAG content with BTC price in output, got %q", got)
	}
	if !strings.Contains(got, "Current context") {
		t.Errorf("expected context header in output, got %q", got)
	}
}

func TestBuildSystemPrompt_DefaultTemplateWithRag(t *testing.T) {
	p := &Prompt{
		Rag:      []string{"Live crypto market prices:\n- BTC: $68000.00"},
		Question: "What is the Bitcoin price?",
	}
	got, err := BuildSystemPrompt(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "BTC: $68000.00") {
		t.Errorf("expected RAG content in default template output, got %q", got)
	}
}
