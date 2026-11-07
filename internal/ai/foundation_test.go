package ai

import (
	"strings"
	"testing"
)

func TestFoundationAcrossWritingPrompts(t *testing.T) {
	for _, p := range []*Prompt{{}, {System: "Return only JSON."}, {System: "You are Malten.", Rag: []string{"source passage"}}, {System: Foundation + "\nSpecialist task"}} {
		got, err := BuildSystemPrompt(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, Foundation) || strings.Count(got, Foundation) != 1 {
			t.Fatal("foundation missing or duplicated")
		}
		if p.System != "" && !strings.Contains(got, p.System) {
			t.Fatal("task instruction lost")
		}
		if len(p.Rag) > 0 && !strings.Contains(got, p.Rag[0]) {
			t.Fatal("source lost")
		}
	}
}
