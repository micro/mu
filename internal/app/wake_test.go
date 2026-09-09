package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestWakeDictationLifecycle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is needed for the voice state test")
	}
	cmd := exec.Command(node, "testdata/wake.js")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wake dictation: %v\n%s", err, out)
	}
}

func TestWakeDictationIsOptInAndDoesNotChangeSpeak(t *testing.T) {
	got := ChatComponent(ChatConfig{Ask: true, Speak: true})
	for _, want := range []string{`id="mu-chat-wake" class="disclosure" hidden`, `aria-pressed="false"`, `Your browser may send audio to its speech provider.`, `then check the words and press Send`, `window.muSay(streamText)`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(ChatComponent(ChatConfig{Ask: true}), `id="mu-chat-wake"`) {
		t.Fatal("wake control drawn where Speak is unavailable")
	}
}
