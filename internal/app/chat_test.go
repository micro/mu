package app

// The shape of a chat.

import (
	"strings"
	"testing"
)

// A conversation puts its input at the bottom; a box puts it at the top.
//
// It was one shape for both. On /agent that left the newest turn at the top,
// the input above it, and everything older stretching away below — which is a
// search box, not a chat. The comment in the script called it reading
// "top-down", which is a reasonable thing to want and is not what a
// conversation is.
func TestATranscriptPutsTheInputUnderTheTurns(t *testing.T) {
	chat := ChatComponent(ChatConfig{Ask: true, Transcript: true, StorageNS: "probe"})

	if !strings.Contains(chat, `class="mu-chat-transcript"`) {
		t.Fatal("a transcript is not marked as one")
	}
	conv := strings.Index(chat, `id="mu-chat-conv"`)
	form := strings.Index(chat, `id="mu-chat-form"`)
	if conv < 0 || form < 0 {
		t.Fatal("the chat has no conversation or no form")
	}
	if form < conv {
		t.Error("the input is above the conversation in a transcript")
	}
	// The turns scroll in their own region and the input sits under them. A
	// sticky input over a scrolling page floats over the message you are
	// reading, and "the bottom" then means the bottom of the document rather
	// than the bottom of the conversation.
	if !strings.Contains(chat, "overflow-y:auto") {
		t.Error("the conversation has no scroll region of its own")
	}
	if strings.Contains(chat, "#mu-chat-form{position:sticky") {
		t.Error("the input is sticky over the conversation")
	}
	// Measured, not guessed. Every constant anybody writes for the height of
	// the page above this is wrong on some screen.
	if !strings.Contains(chat, "function fitConv()") {
		t.Error("nothing measures the region")
	}
	if !strings.Contains(chat, "conv.getBoundingClientRect().top") {
		t.Error("the height is not measured from where the region actually starts")
	}

	// A box keeps the old order: you arrive at it and type into it, and the
	// answer appears underneath. That is Home, and it is right there.
	// The class on the shell, not anywhere — the stylesheet carries the
	// transcript rules either way, so looking for the string is not the
	// question.
	box := ChatComponent(ChatConfig{Ask: true, StorageNS: "probe"})
	if strings.Contains(box, `<div id="mu-chat" class="mu-chat-transcript">`) {
		t.Error("a plain box was marked as a transcript")
	}
	if strings.Index(box, `id="mu-chat-form"`) > strings.Index(box, `id="mu-chat-conv"`) {
		t.Error("a box put its input below the conversation")
	}
}

// Following the answer down, but not when somebody has scrolled up to read
// something. Chasing the bottom then is the thing every chat gets wrong once.
func TestItFollowsTheAnswerButDoesNotChaseIt(t *testing.T) {
	chat := ChatComponent(ChatConfig{Ask: true, Transcript: true, StorageNS: "probe"})
	if !strings.Contains(chat, "function nearBottom()") {
		t.Error("nothing checks whether the reader is at the bottom")
	}
	if !strings.Contains(chat, "if(!force && !nearBottom()) return;") {
		t.Error("the scroll is not conditional on the reader being at the bottom")
	}
	// And it is the conversation that scrolls, not the window.
	if !strings.Contains(chat, "conv.scrollTo(") {
		t.Error("the scroll moves the page rather than the conversation")
	}
	if !strings.Contains(chat, "conv.scrollTop+conv.clientHeight") {
		t.Error("being at the bottom is measured against the page, not the conversation")
	}
}
