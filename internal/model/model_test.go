package model

import (
	"strings"
	"testing"
)

func TestSystemPromptComposition(t *testing.T) {
	m := Model{Foundation: "Foundation", Instructions: "Model instructions", Knowledge: []Source{{Name: "Reference", URL: "https://example.com/private"}}}
	got := m.SystemPrompt("Task instructions")
	if got != "Foundation\n\nModel instructions\n\nTask instructions" {
		t.Fatal(got)
	}
	if strings.Contains(got, "example.com") {
		t.Fatal("source metadata entered system instructions")
	}
	if got := (Model{Instructions: "Only instructions"}).SystemPrompt(""); got != "Only instructions" {
		t.Fatal(got)
	}
}
func TestFoundationIsNotDuplicated(t *testing.T) {
	m := Default()
	original := m.SystemPrompt("Return only JSON.")
	if got := m.SystemPrompt(original); got != original {
		t.Fatal("foundation duplicated", got)
	}
	if got := m.SystemPrompt(""); got != Foundation {
		t.Fatal("empty instructions changed foundation")
	}
}
func TestDefinitionsDoNotShareMutableSources(t *testing.T) {
	first := Default()
	first.Knowledge[0].Name = "Changed"
	second := Default()
	if second.Knowledge[0].Name != "Reminder" {
		t.Fatal("model definitions share mutable sources")
	}
	first.LLM = LLM{Provider: "local", ID: "replacement"}
	if first.SystemPrompt("Task") != second.SystemPrompt("Task") {
		t.Fatal("LLM selection changed foundation")
	}
	if len(second.Services) != 0 {
		t.Fatal("default definition grants service access")
	}
}
