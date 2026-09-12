package home

import (
	"os"
	"strings"
	"testing"
)

func TestLandingConversationSurvivesAccountEntry(t *testing.T) {
	if !strings.Contains(indexBody(), `var NS="landing"`) {
		t.Fatal("landing must retain the guest conversation")
	}
	home, err := os.ReadFile("home.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(home), "agent.HandoffHTML(r)") {
		t.Fatal("login must carry the landing conversation")
	}
}

func TestHomeStartsWithAFreshPrompt(t *testing.T) {
	b, err := os.ReadFile("home.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "StorageNS:") {
		t.Fatal("Home restores browser chat instead of a fresh prompt")
	}
}
