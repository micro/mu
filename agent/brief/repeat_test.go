package brief

// Why the line kept saying the same thing.
//
// gather calls three tools with no arguments, so it reads the top of each feed
// — which at 10am and at 11am is very nearly the same rows. Asked for "the two
// or three most consequential things" from an input that has not moved, a model
// correctly returns the same two or three, and does so every hour of the day.
// The prompt had no memory to know better.

import (
	"strings"
	"testing"
	"time"
)

// The lines already published are held, newest first, and bounded.
func TestTheBriefRemembersWhatItSaid(t *testing.T) {
	mu.Lock()
	entries = nil
	for i, text := range []string{"first", "second", "third", "fourth", "fifth"} {
		entries = append(entries, Entry{Text: text,
			Written: time.Now().Add(time.Duration(i) * time.Minute), Day: today()})
	}
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); entries = nil; mu.Unlock() })

	got := said()
	if len(got) != saidLately {
		t.Fatalf("the model is shown %d past lines, want %d", len(got), saidLately)
	}
	// Newest first: what was said an hour ago is what it is most likely to
	// repeat, so it is the first thing the instruction names.
	if got[0] != "fifth" {
		t.Errorf("the most recent line is %q, want the newest", got[0])
	}
	for _, stale := range []string{"first", "second"} {
		for _, g := range got {
			if g == stale {
				t.Errorf("%q is still being shown; only the last %d are",
					stale, saidLately)
			}
		}
	}
}

// An empty history changes nothing about the question.
//
// The first line of a fresh instance has nothing to avoid repeating, and a
// heading over an empty list is a instruction about nothing that the model has
// to read past.
func TestNothingSaidYetAddsNothingToTheQuestion(t *testing.T) {
	mu.Lock()
	entries = nil
	mu.Unlock()

	if got := said(); len(got) != 0 {
		t.Errorf("a fresh instance has already said %v", got)
	}
}

// And the standing rule is in the prompt, not only in the question.
//
// The list of published lines is today's facts and goes in the question; that
// they are spent is a rule that holds every day and belongs with the other
// rules. Split the other way, the model gets a list with no instruction about
// it on the runs where the question is assembled differently.
func TestThePromptSaysPublishedLinesAreSpent(t *testing.T) {
	if !strings.Contains(system, "already published") {
		t.Error("the system prompt never says what to do with lines it has " +
			"already published, so the rule lives only in the question")
	}
}
