package agent

import (
	"strings"
	"testing"
)

// A model's own tool-call syntax never reaches the reader.
//
// Some models finish a turn by writing the next tool call as ordinary text —
// the delimiters and the arguments, in the reply — rather than emitting it as a
// call. It happens most after a large tool result or a large argument, which is
// exactly what building something looks like: several kilobytes written, and
// then the call that would have hosted it arrives as prose.
//
// The call did not run, so the thing it was going to do has not happened. What
// showing the markup does is tell somebody their agent is broken in a way they
// cannot act on, while hiding the one useful fact — that the work before it did
// run. Saying so plainly is the same turn, honestly reported.
func TestAModelsToolSyntaxIsNotShownAsAnAnswer(t *testing.T) {
	leaked := "Here you go:\n<｜DSML｜tool_calls>\n<｜DSML｜invoke name=\"apps_create\">" +
		"<｜DSML｜parameter name=\"html\">&lt;!doctype html&gt;</｜DSML｜parameter>"

	got := withoutLeakedToolCall(leaked, []string{"shell_write"})
	for _, bad := range []string{"DSML", "tool_calls", "invoke name"} {
		if strings.Contains(got, bad) {
			t.Errorf("protocol markup reached the reader: %q", got)
		}
	}
	// And it says what did run, because that is the part worth knowing.
	if !strings.Contains(got, "shell_write") {
		t.Errorf("the answer does not say what ran: %q", got)
	}

	// With nothing having run, it says that instead of implying progress.
	none := withoutLeakedToolCall(leaked, nil)
	if strings.Contains(none, "What ran") {
		t.Errorf("a turn where no tool ran claims some did: %q", none)
	}
	if !strings.Contains(none, "nothing ran") {
		t.Errorf("a turn that did nothing does not say so: %q", none)
	}

	// An ordinary answer is untouched, including one that talks about tools.
	const plain = "I used the shell to write the file and it works now."
	if withoutLeakedToolCall(plain, []string{"shell_write"}) != plain {
		t.Error("an ordinary answer was rewritten")
	}
	// Including one that legitimately shows HTML.
	const withHTML = "The page starts with <!doctype html> and a <title>."
	if withoutLeakedToolCall(withHTML, nil) != withHTML {
		t.Error("an answer containing markup of its own was rewritten")
	}
}
