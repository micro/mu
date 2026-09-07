package home

import (
	"os"
	"strings"
	"testing"
)

func TestLoginKeepsTheLandingConversation(t *testing.T) {
	landing := indexBody()
	if !strings.Contains(landing, `var NS="landing"`) {
		t.Error("the public conversation is not retained for the login handoff")
	}

	home, err := os.ReadFile("home.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`agent.HandoffHTML(r)`,
	} {
		if !strings.Contains(string(home), want) {
			t.Errorf("Home does not adopt the landing conversation: missing %q", want)
		}
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
