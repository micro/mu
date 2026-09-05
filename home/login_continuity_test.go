package home

import (
	"os"
	"strings"
	"testing"
)

func TestLoginTurnsTheLandingChatIntoTheHomeChat(t *testing.T) {
	landing := indexBody()
	if !strings.Contains(landing, `var NS="landing"`) {
		t.Error("the public conversation is not retained for the login handoff")
	}

	home, err := os.ReadFile("home.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`StorageNS:       "home"`, `ImportNS:        "landing"`} {
		if !strings.Contains(string(home), want) {
			t.Errorf("Home does not adopt the landing conversation: missing %q", want)
		}
	}
}
