package agent

import (
	"strings"
	"testing"

	gmai "go-micro.dev/v6/ai"
)

// A run that does the same kind of thing twice says so twice.
//
// This is the bug behind "it just said thinking the whole time". A watcher
// pairs a tool_start with its tool_end to know how much is still running, and
// it was pairing on the label — which for a build is the same string every
// time. So the second command looked like a repeat of the first and was
// dropped, the in-flight count fell to zero, and the screen sat on the
// between-tools label for the rest of a three-minute run while eight more
// commands went past behind it.
func TestEveryCallIsItsOwnCallEvenWhenTheLabelRepeats(t *testing.T) {
	calls := []gmai.ToolCall{
		{ID: "a", Name: "shell_Server_Run", Input: map[string]any{"command": "mkdir -p site"}},
		{ID: "b", Name: "shell_Server_Run", Input: map[string]any{"command": "ls site"}},
		{ID: "c", Name: "shell_Server_Run", Input: map[string]any{"command": "cat site/index.html"}},
	}

	seen := map[string]bool{}
	for _, c := range calls {
		run, show := toolRun(c)
		if !show {
			t.Fatalf("%s was not shown at all", c.ID)
		}
		if run.ID != c.ID {
			t.Errorf("call %s carries id %q, so a watcher cannot pair it", c.ID, run.ID)
		}
		if seen[run.ID] {
			t.Errorf("two calls share the id %q", run.ID)
		}
		seen[run.ID] = true
	}
	if len(seen) != len(calls) {
		t.Errorf("%d of %d calls were distinguishable", len(seen), len(calls))
	}
}

// And it says which command, because that is the only part that moves.
func TestTheLabelNamesTheCommand(t *testing.T) {
	run, _ := toolRun(gmai.ToolCall{
		ID: "a", Name: "shell_Server_Run",
		Input: map[string]any{"command": "python3 -m http.server 8000"},
	})
	if !strings.Contains(run.Label, "python3 -m http.server") {
		t.Errorf("the label does not say what ran: %q", run.Label)
	}

	wrote, _ := toolRun(gmai.ToolCall{
		ID: "b", Name: "shell_Server_Write",
		Input: map[string]any{"path": "site/index.html", "content": "<!doctype html>"},
	})
	if !strings.Contains(wrote.Label, "site/index.html") {
		t.Errorf("the label does not say what was written: %q", wrote.Label)
	}
	// Not the file's contents, which is the whole page.
	if strings.Contains(wrote.Label, "doctype") {
		t.Errorf("the label carries the file body: %q", wrote.Label)
	}
}

// A command can be a heredoc writing a whole page. One line of it reaches the
// screen, because this is a status line and not a transcript.
func TestALongCommandIsCutToALine(t *testing.T) {
	run, _ := toolRun(gmai.ToolCall{
		ID: "a", Name: "shell_Server_Run",
		Input: map[string]any{"command": "cat > index.html <<'EOF'\n" +
			strings.Repeat("<div class=\"row\">a fairly long line of markup</div>\n", 40) + "EOF"},
	})
	if len(run.Label) > 80 {
		t.Errorf("the label is %d characters, which is not a status line: %q", len(run.Label), run.Label)
	}
	if strings.Contains(run.Label, "\n") {
		t.Errorf("the label spans lines: %q", run.Label)
	}
	if !strings.HasSuffix(run.Label, "…") {
		t.Errorf("a cut label does not say it was cut: %q", run.Label)
	}
}

// With no arguments to show it still says what kind of thing is happening,
// which is what it always did.
func TestWithoutArgumentsItStillSaysTheKind(t *testing.T) {
	run, show := toolRun(gmai.ToolCall{ID: "a", Name: "shell_Server_Run"})
	if !show || run.Label == "" {
		t.Fatal("a call with no arguments shows nothing at all")
	}
	if !strings.Contains(strings.ToLower(run.Label), "command") {
		t.Errorf("the fallback does not say what kind: %q", run.Label)
	}
}

// The tools nobody wants narrated stay quiet.
func TestThePlanToolIsStillSilent(t *testing.T) {
	if _, show := toolRun(gmai.ToolCall{ID: "a", Name: "plan"}); show {
		t.Error("the plan tool is narrated")
	}
}
