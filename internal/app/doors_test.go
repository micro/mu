package app

// The doors sit directly under the input.
//
// Directly, which is the whole reason this is a field on the component rather
// than something each page writes after it. Written after, the row lands below
// the Speak toggle and the suggestion row — near the box rather than under it —
// and the row is the second half of the box's own sentence: ask anything here,
// or search one of these. A control between the two breaks the sentence.

import (
	"strings"
	"testing"
)

func TestTheDoorsAreDirectlyUnderTheInput(t *testing.T) {
	body := ChatComponent(ChatConfig{
		Ask:   true,
		Speak: true,
		Doors: `<a href="/news">News</a>`,
	})

	form := strings.Index(body, `id="mu-chat-form"`)
	doors := strings.Index(body, `id="mu-chat-doors"`)
	opts := strings.Index(body, `id="mu-chat-opts"`)

	if form < 0 || doors < 0 || opts < 0 {
		t.Fatalf("missing a part: form=%d doors=%d opts=%d", form, doors, opts)
	}
	if doors < form {
		t.Error("the doors are drawn above the box")
	}
	if doors > opts {
		t.Error("the Speak toggle sits between the box and the doors, so the row\n" +
			"is near the input rather than under it — which is the whole reason\n" +
			"this is a field on the component rather than markup a page appends")
	}
}

// A transcript keeps its input at the bottom, and the doors go with it.
func TestTheDoorsFollowTheInputInATranscript(t *testing.T) {
	body := ChatComponent(ChatConfig{
		Ask: true, Transcript: true, Doors: `<a href="/news">News</a>`,
	})
	form := strings.Index(body, `id="mu-chat-form"`)
	doors := strings.Index(body, `id="mu-chat-doors"`)
	conv := strings.Index(body, `id="mu-chat-conv"`)
	if form < 0 || doors < 0 || conv < 0 {
		t.Fatalf("missing a part: form=%d doors=%d conv=%d", form, doors, conv)
	}
	if doors < form {
		t.Error("the doors left the input behind when the layout flipped")
	}
	if conv > form {
		t.Error("this is not a transcript: the conversation is not above the input")
	}
}

// Nothing to show is nothing to draw. Every surface already inside one of these
// services passes no doors, and an empty wrapper would still take its margin.
func TestNoDoorsIsNoRow(t *testing.T) {
	for _, doors := range []string{"", "   ", "\n\t"} {
		// The id, not the word: the component's own stylesheet names the row, so
		// a substring match on the name is true whether or not the row is drawn.
		if body := ChatComponent(ChatConfig{Ask: true, Doors: doors}); strings.Contains(body, `<div id="mu-chat-doors">`) {
			t.Errorf("an empty doors row was drawn for %q", doors)
		}
	}
}
