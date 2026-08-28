package code

import (
	"strings"
	"testing"
)

// The page it is working on is in the instructions, not fetched with a tool.
//
// This is the difference between a turn that changes something and one that
// does not, and it is not obvious enough to leave to a comment. Told to read
// the file first, every edit run went the same way: shell_read returned four
// kilobytes as a tool result, and the model then replied with the document as
// prose and changed nothing. A model that has just received a large tool result
// is reliably unable to emit the next tool call.
//
// The same bytes in the instructions cost the same tokens and leave the model
// with one small call to make. Given them there, it edits with sed and the file
// changes.
func TestTheCurrentPageIsGivenNotFetched(t *testing.T) {
	const page = "<!doctype html><title>Tally</title><h1>Tally</h1>"
	got := instructions("apps/tally", true, page)

	if !strings.Contains(got, page) {
		t.Errorf("the page is not in the instructions, so the model has to read "+
			"it — which is the thing that stops the edit landing:\n%s", got)
	}
	if !strings.Contains(got, "do not need to read it") {
		t.Errorf("nothing tells the model it already has the page, so it will "+
			"fetch what it was just given:\n%s", got)
	}
	// A new app has nothing to show, and an empty "this is what it contains
	// now" section is an invitation to edit a file that is not there.
	if fresh := instructions("apps/tally", false, ""); strings.Contains(fresh, "contains now") {
		t.Errorf("a new app's instructions describe a page that does not exist:\n%s", fresh)
	}
}

// A new app is told to write, not to look.
//
// Given only "the page is apps/x/index.html", a run listed the empty directory,
// reported that it was empty, and stopped — which was a reasonable reading. It
// had not been told the file was its to create.
func TestANewAppIsToldToWriteIt(t *testing.T) {
	got := instructions("apps/tally", false, "")
	if !strings.Contains(got, "does not exist yet") {
		t.Errorf("nothing says the file is the model's to create, so a run can "+
			"look at the empty directory and stop:\n%s", got)
	}
}

// Protocol markup never reaches the page.
//
// Some models end a run by emitting another tool call as literal text — the
// delimiters and all — most often right after a large tool result, which is
// precisely when a turn here runs. The change is made and only the sentence
// about it is lost, so showing the markup would report a break that did not
// happen.
func TestLeakedToolMarkupIsNotShownAsAnAnswer(t *testing.T) {
	leaked := "<｜DSML｜tool_calls>\n<｜DSML｜invoke name=\"shell_Server_Run\">sed -i s/a/b/"
	got := account(leaked, []string{"shell_run"})
	if strings.Contains(got, "DSML") || strings.Contains(got, "tool_calls") {
		t.Errorf("raw tool-call markup is shown to the reader as the answer: %q", got)
	}
	if !strings.Contains(got, "shell_run") {
		t.Errorf("the fallback does not say what the turn actually did: %q", got)
	}
	// An ordinary answer is left exactly alone.
	const plain = "Made the background white and the text dark."
	if account(plain, []string{"shell_run"}) != plain {
		t.Error("a normal answer was rewritten")
	}
}

// What somebody asked for becomes the address, without the asking-words.
func TestTheSlugIsWhatWasAskedForNotHowItWasAsked(t *testing.T) {
	for ask, want := range map[string]string{
		"build me a tip calculator":   "tip-calculator",
		"a tip calculator":            "tip-calculator",
		"can you make me a todo list": "todo-list",
	} {
		if got := slugFor(ask); got != want {
			t.Errorf("slugFor(%q) = %q, want %q", ask, got, want)
		}
	}
	// Slugs have a minimum length, and a one-word ask is under it.
	if got := slugFor("a timer"); len(got) < 3 {
		t.Errorf("slugFor produced %q, which apps will refuse", got)
	}
}
