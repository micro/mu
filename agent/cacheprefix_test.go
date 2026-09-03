package agent

// The part of a request that repeats, and why it has to.

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Nothing that changes is in the system prompt.
//
// A provider caches the prefix of a request up to a breakpoint, and the order
// of a request is tools, then system, then messages — so the breakpoint at the
// end of the system prompt is what caches the tool catalogue with it. That
// catalogue is around ten thousand tokens for a signed-in caller: a hundred and
// nineteen tools, re-sent and re-billed on every turn and on every round of a
// tool loop.
//
// It was never cached once, because the system prompt carried an RFC3339
// timestamp and so was unique to the second. A prefix that never repeats never
// matches, and the one place in the request that has to be byte-identical was
// the place the clock was written into.
//
// Asserted against the source. What is being pinned is which half of the
// request a value is assembled into, and only the source says that — a rendered
// prompt would show the same words either way.
func TestTheCachedPrefixDoesNotCarryAClock(t *testing.T) {
	b, err := os.ReadFile("native.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	sys := src[strings.Index(src, "\tsys := "):strings.Index(src, "\t// And the facts, which are what changes.")]
	if sys == "" {
		t.Fatal("cannot find where the system prompt is built — this scan is broken, not the code")
	}
	for _, volatile := range []string{"nowRFC", "today", "nowContext(", "UserContextFunc"} {
		if strings.Contains(sys, volatile) {
			t.Errorf("%s is in the system prompt, which is the cached prefix — every "+
				"request is then a cache miss, and the tool catalogue is re-billed in "+
				"full on every round of every loop", volatile)
		}
	}
}

// And they are all still sent, as the message in front of the question.
func TestWhatChangesIsStillGiven(t *testing.T) {
	brief := briefing([]string{
		"The current date and time is " + time.Now().Format(time.RFC3339) + ".",
		"",
		"User context:\nLives in London",
	})
	m := history(brief, []QueryMessage{{Role: "user", Text: "and then what"}})

	if len(m.msgs) == 0 {
		t.Fatal("no messages at all")
	}
	first := m.msgs[0]
	if first.Role != "user" {
		t.Errorf("the briefing is in the %q role; a model reading its own words as "+
			"something it already said is the alternative", first.Role)
	}
	text, _ := first.Content.(string)
	for _, want := range []string{"current date and time", "Lives in London"} {
		if !strings.Contains(text, want) {
			t.Errorf("the briefing dropped %q — it is not in the system prompt any "+
				"more, so nothing else is carrying it", want)
		}
	}
	// In front of the conversation, because what is true now is true of all of
	// it rather than of its last turn.
	last, _ := m.msgs[len(m.msgs)-1].Content.(string)
	if len(m.msgs) < 2 || !strings.Contains(last, "and then what") {
		t.Errorf("the question is not last: %#v", m.msgs)
	}
}

// Nothing to say is no message, rather than an empty one.
func TestAnEmptyBriefingIsNotSent(t *testing.T) {
	if got := briefing([]string{"", "   "}); got != "" {
		t.Errorf("briefing of nothing is %q", got)
	}
	m := history("", []QueryMessage{{Role: "user", Text: "hello"}})
	if len(m.msgs) != 1 {
		t.Errorf("an empty briefing still took a turn: %#v", m.msgs)
	}
}
