package agent

// /agent answers a program and a page, told apart by what they ask for.

import (
	"os"
	"strings"
	"testing"
)

// One word for one thing.
//
// This was routed on the shape of the body for a commit: the page sent
// {"prompt": …} and the API sent {"text": …}, so the field name decided which
// handler took the call. It worked, and it made an inconsistency load-bearing —
// two names for one idea, now with a rule depending on it.
//
// Everything in this codebase sends an agent a prompt. The page posts one,
// QueryOpts takes one, a task's Run builds one. So the API takes a prompt too,
// and what separates the callers is the honest difference between them:
// handleQuery answers text/event-stream and nothing else, and a program wants
// an answer rather than a stream of one.
func TestOneWordForTheQuestion(t *testing.T) {
	api, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(api), "Prompt string `json:\"prompt\"`") {
		t.Error("the API door does not take a prompt, so what you send an agent\n" +
			"has a different name depending on which door you came through")
	}
	// text stays, because a program that learned it should not find it gone.
	if !strings.Contains(string(api), "Text string `json:\"text,omitempty\"`") {
		t.Error("the old name was removed outright, which breaks every caller\n" +
			"that already learned it")
	}

	h, err := os.ReadFile("agent.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(h)
	if !strings.Contains(body, `strings.Contains(r.Header.Get("Accept"), "text/event-stream")`) {
		t.Error("/agent is not routed on what the caller asks for")
	}
	if strings.Contains(body, "apiShaped") {
		t.Error("the body-shape routing is back — that is a naming inconsistency\n" +
			"promoted to a rule")
	}
}

// prompt wins where both are sent, because it is the name and text is the alias.
func TestPromptIsTheNameAndTextIsTheAlias(t *testing.T) {
	for _, c := range []struct {
		in   apiAsk
		want string
	}{
		{apiAsk{Prompt: "a"}, "a"},
		{apiAsk{Text: "b"}, "b"},
		{apiAsk{Prompt: "a", Text: "b"}, "a"},
		{apiAsk{Prompt: "   ", Text: "b"}, "b"},
		{apiAsk{}, ""},
	} {
		if got := c.in.ask(); got != c.want {
			t.Errorf("%+v.ask() = %q, want %q", c.in, got, c.want)
		}
	}
}

// And the page says which door it is knocking on.
func TestThePageAsksForAStream(t *testing.T) {
	src, err := os.ReadFile("../internal/app/chat.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `'Accept':'text/event-stream'`) {
		t.Error("the chat box does not ask for a stream, so /agent cannot tell it\n" +
			"from a program and answers it with JSON it cannot read")
	}
}
