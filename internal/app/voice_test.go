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

	got := ChatComponent(ChatConfig{Ask: true, StorageNS: "probe", Speak: true})

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

// Speak is not on the front door.
//
// The landing renders the same box with Ask set, and a signed-out ask returns
// 401: the component draws a refusal with two ways on — search the archive, or
// sign in and ask it. So no answer can arrive there, and a toggle controlling
// how one is read out sat under it anyway, on the first screen a stranger
// sees. Reported as exactly that.
//
// The microphone stays, and the asymmetry is the point. It fills the box, and
// the box works: what you dictate is carried to the archive or through the
// sign-in. An input to something that happens, against an output from
// something that cannot.
func TestSpeakIsOnlyWhereAnAnswerCanArrive(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	// The landing: Ask, and nobody signed in.
	front := ChatComponent(ChatConfig{Ask: true, HideSuggestions: true})
	if strings.Contains(front, `id="mu-chat-say"`) {
		t.Errorf("the signed-out landing offers to read an answer out loud, and "+
			"cannot produce one:\n%s", front)
	}
	if !strings.Contains(front, `id="mu-chat-mic"`) {
		t.Error("the microphone went with it — dictating into the box still works, " +
			"and what it types is carried to the archive or through a sign-in")
	}

	// And where somebody can have a conversation, it is there.
	in := ChatComponent(ChatConfig{Ask: true, StorageNS: "probe", Speak: true})
	if !strings.Contains(in, `id="mu-chat-say"`) {
		t.Error("a surface that can answer does not offer to read it out")
	}
}

// Which voice reads an answer.
//
// The utterance was constructed with nothing set — new SpeechSynthesisUtterance(text)
// and straight to speak() — which uses the platform default, and every
// platform's default is its oldest speech engine. That is where "why is it so
// robotic" came from: nothing was choosing, so the OS chose its fallback.
//
// The ranking itself is judged in a browser against the voice lists the
// platforms actually report, because the API says nothing about quality and the
// only signal is the name. What is pinned here is that a choice is made at all
// and that the utterance carries it — the two things whose absence produced the
// original bug and would produce it again silently.
func TestAnAnswerIsReadInAChosenVoice(t *testing.T) {
	js := ChatComponent(ChatConfig{Ask: true, Speak: true})

	if !strings.Contains(js, "u.voice=voice") {
		t.Error("the utterance is spoken without a voice set, so it uses the\n" +
			"platform default — Microsoft David on Windows, the oldest Macintalk\n" +
			"descendant on macOS. That is the robotic one.")
	}
	if !strings.Contains(js, "pickVoice()") {
		t.Error("nothing chooses a voice")
	}
	// The list arrives asynchronously and is usually empty on the first call.
	// Without re-picking, the first answer of a session is read by the default
	// and every one after it by the good one, which is worse than either.
	if !strings.Contains(js, "voiceschanged") {
		t.Error("the voice is chosen once, before the browser has loaded the list —\n" +
			"so the first answer of a session gets the default voice")
	}
}

// Language is a filter and not a preference: a good German voice reading
// English is worse than any English one.
func TestAVoiceInTheWrongLanguageIsNotConsidered(t *testing.T) {
	js := ChatComponent(ChatConfig{Ask: true, Speak: true})
	if !strings.Contains(js, "else continue;") {
		t.Error("a voice whose language does not match is scored rather than\n" +
			"skipped, so a machine with no voice in the page's language reads\n" +
			"the answer in some other one")
	}
}

// macOS ships joke voices — Bells, Bubbles, Deranged, Zarvox — that match a
// language query perfectly well and would be picked by anything filtering on
// language alone.
func TestTheNoveltyVoicesAreExcluded(t *testing.T) {
	js := ChatComponent(ChatConfig{Ask: true, Speak: true})
	for _, name := range []string{"zarvox", "bubbles", "deranged"} {
		if !strings.Contains(js, name) {
			t.Errorf("%q is not excluded — it is a macOS novelty voice that matches\n"+
				"en-US and would be a candidate", name)
		}
	}
}
