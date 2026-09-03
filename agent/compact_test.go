package agent

import (
	"strings"
	"testing"
)

// A conversation too long to carry keeps its beginning as a summary.
//
// Dropping the oldest turns loses the wrong thing: the beginning is where
// somebody says what they are trying to do, so a long working conversation
// forgets its own purpose while remembering the last twenty exchanges of
// detail.
//
// No model is configured in a test binary, so this exercises the fallback —
// which is the path that must never break, because it is what happens on every
// instance with no key and every time the summarising call fails.
func TestATrimmedConversationStillSaysWhatIsMissing(t *testing.T) {
	big := strings.Repeat("x", 20_000)
	turns := []QueryMessage{
		{Role: "user", Text: "I am trying to migrate the billing service"},
		{Role: "assistant", Text: big},
		{Role: "user", Text: big},
		{Role: "assistant", Text: big},
		{Role: "user", Text: "what should I do next?"},
	}

	msgs := history("", turns).Messages()
	first, _ := msgs[0].Content.(string)

	if !strings.HasPrefix(first, "[") {
		t.Fatalf("the conversation starts with %q, so nothing stands in for what "+
			"was dropped", clip(first))
	}
	// Case-insensitively, because the assertion is about what the line means
	// and there are two lines that mean it: a summarised stand-in opens
	// "[Earlier in this conversation…", the model-less fallback does not
	// capitalise. This only ever ran the second on a box with no key, so a
	// machine with one failed on the capital E.
	if !strings.Contains(strings.ToLower(first), "earlier") {
		t.Errorf("the opening message %q does not say it stands in for earlier "+
			"turns", clip(first))
	}
	// The newest turn is still last, whatever went in front.
	last, _ := msgs[len(msgs)-1].Content.(string)
	if last != "what should I do next?" {
		t.Errorf("the newest turn is %q", clip(last))
	}
}

// Summarising is skipped rather than attempted when there is no model, and a
// failure is never fatal.
func TestSummarisingNeverFailsTheQuestion(t *testing.T) {
	noProviders(t)

	if got := summarise([]QueryMessage{{Role: "user", Text: "something"}}); got != "" {
		t.Errorf("summarise returned %q with no model configured; it should decline "+
			"so the caller falls back to the note", clip(got))
	}
	if got := summarise(nil); got != "" {
		t.Errorf("summarising nothing produced %q", clip(got))
	}
}

// What goes to the summariser is the dropped turns, oldest first, bounded.
//
// A summary of a summary is fine; a summarising request that itself blows the
// context is not.
func TestTheTranscriptIsBoundedAndInOrder(t *testing.T) {
	var turns []QueryMessage
	for i := 0; i < 200; i++ {
		turns = append(turns,
			QueryMessage{Role: "user", Text: strings.Repeat("q", 2_000)},
			QueryMessage{Role: "assistant", Text: strings.Repeat("a", 2_000)})
	}

	text, covered := transcriptOf(turns)
	if len(text) > compactionLimit+4_000 {
		t.Errorf("the transcript is %d characters against a limit of %d",
			len(text), compactionLimit)
	}
	if covered == 0 || covered > len(turns) {
		t.Errorf("covered %d of %d turns", covered, len(turns))
	}

	// Oldest first, so it reads as a conversation.
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 {
		t.Fatalf("got %d lines", len(lines))
	}
	// The last line is the newest turn that fit, and the trimming takes from
	// the front — so what survives ends at the most recent dropped turn.
	if !strings.HasPrefix(lines[len(lines)-1], "Assistant: ") {
		t.Errorf("the transcript ends with %q, not the newest dropped turn",
			clip(lines[len(lines)-1]))
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "User: ") && !strings.HasPrefix(l, "Assistant: ") {
			t.Errorf("a line is not attributed to a speaker: %q", clip(l))
			break
		}
	}
}

// Empty turns do not become empty lines in the transcript.
func TestTheTranscriptSkipsEmptyTurns(t *testing.T) {
	text, covered := transcriptOf([]QueryMessage{
		{Role: "user", Text: "real"},
		{Role: "assistant", Text: "   "},
		{Role: "user", Text: "also real"},
	})
	if covered != 2 {
		t.Errorf("covered %d turns, want 2", covered)
	}
	if strings.Contains(text, "Assistant: \n") {
		t.Error("an empty turn became a line in the transcript")
	}
}

// The same turns produce the same key, so a growing conversation pays for a
// compaction once rather than once per question.
func TestTheSameTurnsAreSummarisedOnce(t *testing.T) {
	a, _ := transcriptOf([]QueryMessage{{Role: "user", Text: "one"}, {Role: "assistant", Text: "two"}})
	b, _ := transcriptOf([]QueryMessage{{Role: "user", Text: "one"}, {Role: "assistant", Text: "two"}})
	if compactionKey(a) != compactionKey(b) {
		t.Error("the same conversation keys differently, so every question pays " +
			"for its own summary")
	}
	c, _ := transcriptOf([]QueryMessage{{Role: "user", Text: "one"}, {Role: "assistant", Text: "three"}})
	if compactionKey(a) == compactionKey(c) {
		t.Error("different conversations share a key, so one would be served the " +
			"other's summary")
	}
}
