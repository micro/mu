package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestChatCompletionAndRecovery(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is needed for the browser response state test")
	}
	src, err := os.ReadFile("chat.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "function ask(q){")
	end := -1
	if start >= 0 {
		if offset := strings.Index(text[start:], "form.addEventListener('submit',"); offset >= 0 {
			end = start + offset
		}
	}
	if start < 0 || end <= start {
		t.Fatal("chat function not found")
	}
	body, _ := json.Marshal(text[start:end])
	cmd := exec.Command(node, "testdata/chat_completion.js")
	cmd.Stdin = strings.NewReader(string(body))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("completion test: %v\n%s", err, out)
	}
}
