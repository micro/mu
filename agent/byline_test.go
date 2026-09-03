package agent

// Who answered.
//
// An agent's reply was a bare card with no name on it. On a page about one
// agent that is survivable; in the inbox, on Home, and in a conversation
// reopened from /recall it is not, because several agents and the platform
// itself write into cards of the same shape. Mu is the place and the agent is
// who answers — the second half of that was never on screen.

import (
	"os"
	"strings"
	"testing"

	"mu/internal/thread"
)

func TestAnAnswerSaysWhoWroteIt(t *testing.T) {
	got := renderTurn(thread.Message{Role: thread.RoleAgent, Text: "it is raining"}, "Micro")
	if !strings.Contains(got, `class="mu-by"`) || !strings.Contains(got, "Micro") {
		t.Errorf("an answer arrives with no name on it: %q", got)
	}
	// Above the answer, not inside it: it is a label over a block.
	if strings.Index(got, "mu-by") > strings.Index(got, "it is raining") {
		t.Errorf("the name is under the answer rather than over it: %q", got)
	}
}

// What you said needs no byline. You know who you are, and a name over your own
// message is the shape of somebody else talking.
func TestYourOwnTurnHasNoByline(t *testing.T) {
	got := renderTurn(thread.Message{Role: thread.RolePerson, Text: "hello"}, "Micro")
	if strings.Contains(got, "mu-by") {
		t.Errorf("your own message is bylined: %q", got)
	}
}

// No name is no byline, and never a guessed one.
//
// A wrong name is worse than none: the whole property here is that what is on
// screen says who actually wrote it.
func TestAnUnknownAuthorIsNotInvented(t *testing.T) {
	got := renderTurn(thread.Message{Role: thread.RoleAgent, Text: "hi"}, "")
	if strings.Contains(got, "mu-by") {
		t.Errorf("an answer from nobody named somebody: %q", got)
	}
	if strings.Contains(got, "Micro") {
		t.Errorf("an answer from nobody was attributed to the default agent: %q", got)
	}
}

// An agent's name is somebody's own text where the agent is theirs.
func TestTheBylineIsEscaped(t *testing.T) {
	got := renderTurn(thread.Message{Role: thread.RoleAgent, Text: "hi"}, `<script>x</script>`)
	if strings.Contains(got, "<script>") {
		t.Errorf("an agent name went into the transcript as markup: %q", got)
	}
}

// And the live reply carries the same byline as the reloaded one.
//
// The transcript is rendered here and a reply arriving now is built in the
// browser — two renderers for one thing, which is exactly how a conversation
// comes to look different above and below a page refresh. This is the cheapest
// guard on that: the browser half has to exist and has to use the same class.
func TestTheLiveReplyIsBylinedToo(t *testing.T) {
	b, err := os.ReadFile("../internal/app/chat.go")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, want := range []string{"AGENT_NAME=", `className='mu-by'`, "function agentName()"} {
		if !strings.Contains(js, want) {
			t.Errorf("the chat component does not byline a live reply: no %q", want)
		}
	}
	// And it prefers the picker, so changing who answers changes the name.
	if !strings.Contains(js, "mu-chat-agent-pick") {
		t.Error("the byline ignores the agent picker, so it names the wrong agent " +
			"as soon as somebody chooses one")
	}
}
