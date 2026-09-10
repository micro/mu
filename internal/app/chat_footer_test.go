package app

import (
	"strings"
	"testing"
)

func TestChatFooterFollowsConversation(t *testing.T) {
	for _, transcript := range []bool{false, true} {
		body := ChatComponent(ChatConfig{Ask: true, Transcript: transcript, FooterHTML: `<nav id="service-launcher">Services</nav>`})
		conversation := strings.Index(body, `id="mu-chat-conv"`)
		footer := strings.Index(body, `id="service-launcher"`)
		if conversation < 0 || footer < conversation {
			t.Fatal("services interrupt the conversation")
		}
	}
}
