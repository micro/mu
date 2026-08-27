package agent

// A run outlives the browser that started it.
//
// You ask something, close the tab, come back later and the answer is there.
// That is how everybody expects a chat with an agent to behave now, and it is
// how this behaves — but only because nobody has threaded a request context
// into the run. It works by omission, which is the kind of property that
// disappears in a refactor nobody thought was risky.
//
// So it is written down. What the tests below hold is the shape rather than the
// plumbing: the answer is recorded on the conversation by the code that
// produced it, not by the code that was streaming it to somebody.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mu/internal/thread"
)

// The SSE handler must not pass the request's context into the agent.
//
// http.Request's context is cancelled the moment the client disconnects. Give
// it to the model call and closing the tab kills the run mid-answer: nothing is
// recorded, the workflow stays "running" for ever, and the person who came back
// to read it finds their own question and silence.
//
// Reading the source rather than simulating a disconnect, because what matters
// is the rule — no request-scoped cancellation reaches a run — and that is a
// property of the code, not of one request.
func TestARunIsNotTiedToTheRequest(t *testing.T) {
	for _, name := range []string{"agent.go", "native.go", "ask.go"} {
		b, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		src := stripGoComments(string(b))
		if strings.Contains(src, "r.Context()") || strings.Contains(src, "req.Context()") {
			t.Errorf("%s passes the request's context into the agent. That context is "+
				"cancelled when the browser disconnects, so closing the tab would kill "+
				"the run: no answer recorded, the workflow stuck at running, and the "+
				"person who came back to read it finds their own question and silence.",
				name)
		}
	}
}

// The answer is written to the record by the run, not by the stream.
//
// Answered is what puts it on the conversation, and it is called after the
// model has finished rather than as tokens go out — so a stream nobody is
// reading still ends with the answer stored. If this ever moves inside the
// token loop, an interrupted stream becomes a lost answer.
func TestTheAnswerIsRecordedByTheRunNotTheStream(t *testing.T) {
	b, err := os.ReadFile("agent.go")
	if err != nil {
		t.Fatal(err)
	}
	src := stripGoComments(string(b))
	i := strings.Index(src, "Answered(accountID, threadID, answer")
	if i < 0 {
		t.Fatal("nothing records the answer in the streaming path any more")
	}
	// The call sits after the streaming is done with — between it and the
	// preceding stream_token there should be no loop.
	before := src[:i]
	if last := strings.LastIndex(before, "stream_token"); last >= 0 {
		if strings.Contains(before[last:], "for ") {
			t.Error("Answered is inside a token loop, so an interrupted stream would " +
				"record a partial answer or none")
		}
	}
}

// And the record itself keeps it: an answer written down is readable by
// whatever opens the conversation next, which is what "come back and it is
// there" actually means.
func TestAnAnswerIsThereWhenYouComeBack(t *testing.T) {
	const who = "durable_reader"
	th := thread.Open(who, thread.WebClient, "durable_1")
	if th == nil {
		t.Fatal("no conversation")
	}
	Said(who, th.ID, "what is the weather", "", "")
	// The browser goes away here. The run does not.
	Answered(who, th.ID, "Cloudy, 14°C.", "flow_1")

	msgs := thread.Messages(who, th.ID, 10)
	if len(msgs) != 2 {
		t.Fatalf("%d messages on the conversation, want the question and the answer", len(msgs))
	}
	var answered bool
	for _, m := range msgs {
		if m.Role == thread.RoleAgent && strings.Contains(m.Text, "14°C") {
			answered = true
		}
	}
	if !answered {
		t.Error("the answer is not on the conversation, so coming back shows the " +
			"question and nothing else")
	}
}

// stripGoComments removes comments so a rule about code is not tripped by prose
// describing it — this file's own doc comment names r.Context().
func stripGoComments(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); {
		switch {
		case strings.HasPrefix(src[i:], "//"):
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				return b.String()
			}
			i += j
		case strings.HasPrefix(src[i:], "/*"):
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			i += j + 4
		default:
			b.WriteByte(src[i])
			i++
		}
	}
	return b.String()
}
