package app

// Talking to it, and being talked back to.
//
// Both halves are the browser's own — SpeechRecognition and speechSynthesis —
// which means the only thing that can go wrong on our side is drawing a control
// where the browser has nothing behind it. Firefox has no SpeechRecognition at
// all, so a microphone button that renders visible on the server would be a
// button that does nothing on a third of the web. Both ship `hidden` and the
// script unhides each one only after finding the thing it drives.
//
// That is the property worth a test, because it is invisible in a browser that
// happens to support both.

import (
	"strings"
	"testing"
)

func TestTheVoiceControlsAreHiddenUntilTheBrowserHasAVoice(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{Ask: true, StorageNS: "probe"})

	for _, want := range []struct{ id, feature string }{
		{"mu-chat-mic", "SpeechRecognition"},
		{"mu-chat-say", "speechSynthesis"},
	} {
		open := strings.Index(got, `id="`+want.id+`"`)
		if open < 0 {
			t.Fatalf("#%s is not on the page, so there is no way to use %s", want.id, want.feature)
		}
		// The element's own attributes: from its id to the end of that tag.
		end := strings.Index(got[open:], ">")
		if end < 0 {
			t.Fatalf("#%s's tag never closes", want.id)
		}
		if !strings.Contains(got[open:open+end], "hidden") {
			t.Errorf("#%s renders visible. A browser without %s draws a control that "+
				"does nothing when pressed, which is worse than not drawing it.",
				want.id, want.feature)
		}
		if !strings.Contains(got, want.feature) {
			t.Errorf("nothing on the page mentions %s, so #%s is hidden forever",
				want.feature, want.id)
		}
	}

	// Dictation fills the box; it does not send. Sending on the last word means
	// watching your own mistranscription go out, and the box is the place you
	// would have caught it.
	if strings.Contains(got, "rec.onend=function(){ask(") || strings.Contains(got, "onspeechend=submit") {
		t.Error("the microphone sends when it stops listening — dictation is wrong " +
			"often enough that the send has to stay a decision")
	}

	// It reads the text, not the markup it was rendered into.
	if !strings.Contains(got, "window.muSay(streamText)") {
		t.Error("muSay is not given the plain text of the answer — a voice reading " +
			"markup says \"less than div\" at you")
	}
}

// A page with nothing to suggest still runs its own JavaScript.
//
// Found by rendering the box on its own to look at the microphone: the mic and
// the Speak toggle stayed hidden, and the console said "Cannot read properties
// of null". SUGGEST was the literal null — json.Marshal of a nil slice — and
// showSuggestions() calls forEach on it near the top of the block, so
// everything below stopped: the agent picker, the poll for an answer that
// landed while the page was gone, window.muChatAsk, and the voice controls.
//
// Home passes HideSuggestions, which returns before the forEach and is why
// nobody saw this. /agent does not: it fills Suggestions from the agent's own
// examples, and an agent somebody made has none.
func TestABoxWithNoSuggestionsStillHasWorkingJavaScript(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{Ask: true, StorageNS: "probe"})
	if strings.Contains(got, "var SUGGEST=null") {
		t.Error("SUGGEST is null, so showSuggestions() throws and every line of " +
			"script after it — the agent picker, the pending-answer poll, " +
			"muChatAsk, the voice controls — never runs")
	}
	if !strings.Contains(got, "var SUGGEST=[]") {
		t.Error("no empty suggestion list; a page that suggests nothing should say so " +
			"as a list with nothing in it")
	}
}
