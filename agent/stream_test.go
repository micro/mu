package agent

// Streaming is an option for the caller, not a fork in the implementation.
//
// It existed already — StreamHooks and a StreamAsk loop — and was reachable
// only from the web's SSE handler, which took an http.ResponseWriter. So the
// one client that needed it first kept it, and the other four could not show an
// answer arriving even where their protocol allows it.
//
// The danger in fixing that is fixing it twice: a streaming path and a quiet
// path that answer the same question differently. These pin the single path.

import (
	"os"
	"strings"
	"testing"
)

func TestNobodyListeningIsNotAStream(t *testing.T) {
	if (StreamHooks{}).wants() {
		t.Error("a caller with no hooks is treated as asking for a stream, so every " +
			"run takes the streaming path whether or not anything can show it")
	}
	for _, h := range []StreamHooks{
		{Token: func(string) {}},
		{ToolStart: func(string, string) {}},
		{ToolEnd: func(string, string) {}},
	} {
		if !h.wants() {
			t.Error("a caller with hooks is not treated as listening")
		}
	}
}

// One function runs the native agent, whoever is watching.
//
// queryNative and streamNative were the same function twice: same
// construction, same post-processing, same contract, differing only in which
// ask they called. Two copies of that is how the web came to skip the router
// while everyone else did not, and nobody noticed because both answers were
// plausible.
func TestThereIsOneNativeRunner(t *testing.T) {
	b, err := os.ReadFile("native.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if strings.Contains(src, "func queryNative(") || strings.Contains(src, "func streamNative(") {
		t.Error("the native agent is run by two functions again — they share their " +
			"construction and post-processing, so they will drift")
	}
	if !strings.Contains(src, "func runNative(") {
		t.Fatal("runNative is gone")
	}
	// The construction happens once. Counting calls, not the declaration —
	// which is what made this fail the first time it was written.
	if n := strings.Count(src, "= buildNativeAgent("); n != 1 {
		t.Errorf("buildNativeAgent is called from %d places, want 1", n)
	}
}

// The web's SSE handler translates events and owns nothing else.
func TestTheSSEHandlerOnlyTranslates(t *testing.T) {
	b, err := os.ReadFile("agent.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, "runNative(accountID, prompt, sopts)") {
		t.Error("the SSE handler no longer goes through the shared runner")
	}
	// Hooks travel in the options, so a client cannot be handed a different
	// runner along with them.
	if !strings.Contains(src, "sopts.Stream = StreamHooks{") {
		t.Error("the SSE handler builds its stream some other way than through " +
			"QueryOpts, which is how it came to be the only client that could")
	}
}

// And every client can ask for it.
func TestAskOffersStreamingToAnyClient(t *testing.T) {
	b, err := os.ReadFile("ask.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, "Stream StreamHooks") {
		t.Error("AskRequest has no Stream, so streaming is still the web's alone")
	}
	if !strings.Contains(src, "Stream:  r.Stream") {
		t.Error("Ask does not pass the caller's hooks down, so setting them does nothing")
	}
}
