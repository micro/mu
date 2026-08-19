package app

// The card on /account for Discord, Telegram and WhatsApp is Clients.
//
// It was headed "Chat", which on this instance is something else: chat is the
// service behind /chat, the live discussion rooms attached to an item. Two
// unrelated things under one word, and the one on /account was not the one
// with a page. The code has called these clients all along — client/discord,
// client/telegram, client/whatsapp — so the heading says what they are.

import (
	"os"
	"strings"
	"testing"
)

func TestTheChannelCardIsCalledClientsNotChat(t *testing.T) {
	src, err := os.ReadFile("../../account/pages.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// Named through app.Section now rather than written as an <h4>, which is
	// why this looks for the call and not the markup. The heading is the point
	// either way.
	if strings.Contains(body, `app.Section("Chat"`) || strings.Contains(body, "<h4>Chat</h4>") {
		t.Error("/account still has a card headed Chat, which reads as the chat " +
			"service rather than the bots you link an account to")
	}
	// Both states of the card — before a code is generated and after — carry
	// the heading, so renaming one and not the other is the likely slip.
	if n := strings.Count(body, `app.Section("Clients"`); n != 2 {
		t.Errorf("found %d Clients sections, want 2 (with a link code and without)", n)
	}
}
