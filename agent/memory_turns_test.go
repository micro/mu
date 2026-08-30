package agent

import (
	"strings"
	"testing"
)

// History reaches the model as turns, in the roles they happened in.
func TestHistoryKeepsItsRoles(t *testing.T) {
	m := history([]QueryMessage{
		{Role: "user", Text: "what is the weather in London"},
		{Role: "assistant", Text: "It is 14°C and raining."},
		{Role: "user", Text: "and tomorrow?"},
	})

	msgs := m.Messages()
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}
	for i, want := range []string{"user", "assistant", "user"} {
		if msgs[i].Role != want {
			t.Errorf("message %d is %q, want %q — an assistant turn quoted inside "+
				"a user message is something the user reported, not something the "+
				"model said", i, msgs[i].Role, want)
		}
	}
}

// An assistant turn is not truncated.
//
// It was cut to 300 characters, so "what was the third thing you listed?" could
// not be answered: the list had been cut off before the model ever saw it. That
// was not a limit anybody chose — it is what you write when history is a string.
func TestAnAssistantTurnIsNotCutOff(t *testing.T) {
	long := "item one. " + strings.Repeat("filler so this runs well past three hundred characters. ", 20) + "item three is the last one."

	m := history([]QueryMessage{
		{Role: "user", Text: "list three things"},
		{Role: "assistant", Text: long},
	})

	got, _ := m.Messages()[1].Content.(string)
	if len(got) != len(long) {
		t.Errorf("the assistant turn arrived %d characters long, was %d", len(got), len(long))
	}
	if !strings.HasSuffix(got, "item three is the last one.") {
		t.Error("the end of the assistant's own answer never reaches the model, so " +
			"it cannot be asked about what it said")
	}
}

// Nothing is written back.
//
// go-micro's Ask adds the question to memory and then reads memory back, while
// the providers build their request as Messages followed by Prompt — so a
// memory that accepted writes would send the question twice, which is exactly
// what the flattened blob was doing with the whole conversation.
func TestTheQuestionIsNotCountedTwice(t *testing.T) {
	m := history([]QueryMessage{{Role: "user", Text: "first"}})

	before := len(m.Messages())
	m.Add("user", "the question being asked right now")
	if after := len(m.Messages()); after != before {
		t.Errorf("memory grew from %d to %d — the question goes to the provider as "+
			"the prompt as well, so accepting the write sends it twice", before, after)
	}

	m.Clear()
	if len(m.Messages()) != before {
		t.Error("Clear discarded the conversation the agent was given")
	}
}

// Empty turns are dropped rather than sent as empty messages, which some
// providers reject outright.
func TestEmptyTurnsAreDropped(t *testing.T) {
	m := history([]QueryMessage{
		{Role: "user", Text: "something"},
		{Role: "assistant", Text: ""},
		{Role: "user", Text: "something else"},
	})
	if n := len(m.Messages()); n != 2 {
		t.Errorf("got %d messages, want 2 — an empty turn is not a turn", n)
	}
}

// A role nothing recognises becomes the user's rather than being sent as-is.
func TestAnUnknownRoleIsNotSentToTheProvider(t *testing.T) {
	m := history([]QueryMessage{{Role: "system", Text: "hello"}, {Role: "", Text: "hi"}})
	for i, msg := range m.Messages() {
		if msg.Role != "user" && msg.Role != "assistant" {
			t.Errorf("message %d has role %q, which no provider accepts", i, msg.Role)
		}
	}
}

// Nil and empty are safe: a first message has no history.
func TestNoHistoryIsNotAnError(t *testing.T) {
	if n := len(history(nil).Messages()); n != 0 {
		t.Errorf("nil history produced %d messages", n)
	}
	var m *threadMemory
	if m.Messages() != nil {
		t.Error("a nil memory did not answer nil")
	}
}

// A long conversation is carried, not cut to three exchanges.
//
// The window was six messages, which is not a conversation — it is the last
// thing you said. Anything from four exchanges ago was gone, so the agent could
// not be asked about its own earlier answer and could not be corrected twice
// about the same thing.
func TestALongConversationSurvives(t *testing.T) {
	var turns []QueryMessage
	for i := 0; i < 60; i++ {
		turns = append(turns,
			QueryMessage{Role: "user", Text: "question " + itoa(i)},
			QueryMessage{Role: "assistant", Text: "answer " + itoa(i)})
	}

	msgs := history(turns).Messages()
	if len(msgs) != len(turns) {
		t.Fatalf("kept %d of %d messages; 120 short turns are nowhere near the budget",
			len(msgs), len(turns))
	}
	if got, _ := msgs[0].Content.(string); got != "question 0" {
		t.Errorf("the conversation starts at %q, so the opening was dropped", got)
	}
}

// What bounds the prompt is size, because size is what is being spent.
//
// A turn is not a unit of anything: one is "yes", the next is forty lines with
// a table in it. Counting turns meant picking a cap low enough to survive the
// worst turn and applying it to all of them.
func TestTheBudgetIsSizeNotTurnCount(t *testing.T) {
	// What a long conversation is trimmed to with nothing to summarise it —
	// so the box this runs on must have no model, whatever the developer has
	// exported. See noProviders.
	noProviders(t)
	big := strings.Repeat("x", 20_000)
	turns := []QueryMessage{
		{Role: "user", Text: "the opening question"},
		{Role: "assistant", Text: big},
		{Role: "user", Text: big},
		{Role: "assistant", Text: big},
		{Role: "user", Text: "the newest question"},
	}

	msgs := history(turns).Messages()

	spent := 0
	for _, m := range msgs {
		s, _ := m.Content.(string)
		spent += len(s)
	}
	if spent > historyBudget+1_000 {
		t.Errorf("carried %d characters against a budget of %d", spent, historyBudget)
	}

	// The newest turn survives: a conversation that will not fit loses its
	// beginning, not its end.
	last, _ := msgs[len(msgs)-1].Content.(string)
	if last != "the newest question" {
		t.Errorf("the newest turn is %q — the tail is what the question is about", clip(last))
	}

	// Nothing is half a turn. Half an answer reads as something the model said
	// and stops mid-sentence, which is the mistake the 300-character
	// truncation made on every turn.
	for i, m := range msgs {
		s, _ := m.Content.(string)
		if strings.Contains(s, "x") && len(s) != len(big) {
			t.Errorf("message %d is %d characters, a fragment of a %d-character turn",
				i, len(s), len(big))
		}
	}

	// And it says something is missing rather than pretending the conversation
	// began where it was cut.
	first, _ := msgs[0].Content.(string)
	if !strings.Contains(first, "not") || !strings.Contains(first, "earlier") {
		t.Errorf("nothing tells the model the conversation was trimmed; it starts %q",
			clip(first))
	}
}

// A single turn larger than the whole budget is still carried, because the
// alternative is a question with no context at all.
func TestOneEnormousTurnIsNotDroppedEntirely(t *testing.T) {
	huge := strings.Repeat("y", historyBudget*2)
	msgs := history([]QueryMessage{{Role: "user", Text: huge}}).Messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want the one turn there is", len(msgs))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func clip(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}

// And the flattening is gone from the native path rather than merely unused.
//
// The check is on the source because the failure it guards is a reintroduction:
// the blob is easy to write, reads as helpful, and costs double the input
// tokens for a worse conversation.
func TestTheNativePathDoesNotFlattenHistory(t *testing.T) {
	src := readSource(t, "native.go")
	if strings.Contains(src, "Conversation so far") {
		t.Error("native.go builds the conversation as prose again; history goes to " +
			"the model as turns — see memory.go")
	}
	if !strings.Contains(src, "gmagent.WithMemory(history(opts.History))") {
		t.Error("the native agent is not given the conversation at all, so every " +
			"question arrives with no context")
	}
}
