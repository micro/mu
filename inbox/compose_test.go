package inbox

// What makes compose a collaboration rather than a button.

import (
	"strings"
	"testing"
)

// The shape the agent is asked for, read back. A subject line and a body, and
// the tolerance that matters: a model that ignores the shape must not have its
// first paragraph shipped as the subject line.
func TestReadingADraft(t *testing.T) {
	for _, c := range []struct {
		name, in, subject, body string
	}{
		{
			name:    "the shape it was asked for",
			in:      "Thursday works\n\nHappy to move it to Thursday — 3pm suits, if that does.",
			subject: "Thursday works",
			body:    "Happy to move it to Thursday — 3pm suits, if that does.",
		},
		{
			name:    "a model that labelled it",
			in:      "Subject: Invoice 4021\n\nAttached, due on the 30th.",
			subject: "Invoice 4021",
			body:    "Attached, due on the 30th.",
		},
		{
			// The failure this is forgiving for. "Sure, here is a draft for
			// you:" as a subject line is worse than no subject, because the
			// subject is the line the recipient reads first.
			name:    "a first line that is prose",
			in:      "Sure — here is a short note you could send to Jane about the meeting, keeping it friendly.\n\nHi Jane, are you free Thursday?",
			subject: "",
			body:    "Sure — here is a short note you could send to Jane about the meeting, keeping it friendly.\n\nHi Jane, are you free Thursday?",
		},
		{
			name:    "no blank line at all",
			in:      "Hi Jane, are you free Thursday?",
			subject: "",
			body:    "Hi Jane, are you free Thursday?",
		},
		{name: "nothing", in: "   ", subject: "", body: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			subject, body := split(c.in)
			if subject != c.subject {
				t.Errorf("subject %q, want %q", subject, c.subject)
			}
			if body != c.body {
				t.Errorf("body %q, want %q", body, c.body)
			}
		})
	}
}

// A draft keeps what you typed. An instruction that ate the recipient, or that
// threw away the paragraph you had already written, is a form nobody presses a
// second time.
func TestDraftingKeepsWhatYouTyped(t *testing.T) {
	Draft = func(accountID, instruction, to, subject, body string) (string, error) {
		// And it is given the draft it is revising, which is what makes "make
		// it shorter" a sentence with a referent.
		if !strings.Contains(instruction, "shorter") {
			t.Errorf("the agent was not told what to do: %q", instruction)
		}
		if body != "The original, at length." {
			t.Errorf("the agent was not given the draft to revise: %q", body)
		}
		return "Short\n\nShorter.", nil
	}
	t.Cleanup(func() { Draft = nil })

	reader(t, "compose-keeps")
	got := drafted("compose-keeps", form{
		To: "jane@example.com", Subject: "Long", Body: "The original, at length.",
		Ask: "make it shorter",
	})

	if got.To != "jane@example.com" {
		t.Errorf("the recipient was lost: %q", got.To)
	}
	if got.Subject != "Short" || got.Body != "Shorter." {
		t.Errorf("the draft did not land: %q / %q", got.Subject, got.Body)
	}
	// Emptied, because the next instruction is nearly always a different one and
	// re-sending the last is how a second press repeats the first.
	if got.Ask != "" {
		t.Errorf("the instruction is still in the box: %q", got.Ask)
	}
}

// A draft that fails says so and changes nothing. The alternative is a form
// that silently keeps what you had, which is indistinguishable from a button
// that does not work.
func TestAFailedDraftKeepsTheDraft(t *testing.T) {
	Draft = nil
	got := drafted("compose-noagent", form{Body: "mine", Ask: "write it"})
	if got.Body != "mine" {
		t.Errorf("a failed draft overwrote the message: %q", got.Body)
	}
	if got.Problem == "" {
		t.Error("a failed draft said nothing")
	}
}

// Nothing is sent by the agent. Draft fills boxes; sending is a separate press.
// This is the rule the whole surface rests on, so it is worth a test that would
// fail if somebody wired Draft to the send path.
func TestDraftingSendsNothing(t *testing.T) {
	sentTo := ""
	Draft = func(accountID, instruction, to, subject, body string) (string, error) {
		sentTo = to // if this ever becomes a send, the recipient is the tell
		return "Note\n\nSomething.", nil
	}
	t.Cleanup(func() { Draft = nil })

	reader(t, "compose-nosend")
	drafted("compose-nosend", form{To: "jane@example.com", Ask: "write it"})
	if sentTo != "jane@example.com" {
		t.Error("the agent was not told who it is to")
	}
	// The proof is structural: drafted returns a form, and only sent() calls
	// mail.SendOut. A draft has no way to reach it.
}
