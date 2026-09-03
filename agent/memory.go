package agent

// The conversation as the model should see it: turns, in the roles they
// happened in.
//
// # What this replaces
//
// The history went to the model as prose inside a single user message:
//
//	Conversation so far:
//	User: what is the weather in London
//	Assistant: It is 14°C and raining.
//
//	New message: and tomorrow?
//
// Three things were wrong with that, and they compound.
//
// The model never saw its own prior turns as its own. An assistant turn quoted
// inside a user message is something the user is *reporting*, not something the
// model said, and every provider treats the distinction as load-bearing.
//
// Assistant turns were truncated to 300 characters. So "what was the third
// thing you listed?" was unanswerable by construction: the list had been cut
// off before the model ever saw it. That is not a limit anybody chose for a
// reason — it is what you write when history is a string and you are worried
// about its length.
//
// And the whole thing was sent twice. go-micro's Ask adds the question to
// memory and *then* reads memory back, while the providers build their request
// as req.Messages followed by req.Prompt — so the blob arrived as the last
// history message and again as the prompt. Every turn paid input tokens for
// its entire context, doubled.
//
// # Why Add does nothing
//
// This memory is read-only on purpose, and that is the fix for the third
// problem rather than a shortcut around it.
//
// Mu's memory is internal/thread. It is written on every turn from every
// client by the machinery, not by a decision the agent takes — see "The system
// of record is not a service" in AGENTS.md. A go-micro agent here is built for
// one question and thrown away after it, so anything it wrote to a memory of
// its own would be discarded unread. Accepting those writes would only put the
// current question back into the history it is being asked against.
//
// So: history in, nothing out. The record is somebody else's job and always
// was.
//
// # Why not go-micro's own memory
//
// The default is store-backed and keyed by agent name, which would be right
// for a long-lived agent that owns its conversation. Mu's agents are named
// uniquely per request precisely so that no provider can carry state between
// two unrelated questions, and the conversation they belong to is a Mu thread
// that predates any of them. Seeding is the honest shape: the agent is handed
// what was said, and does not accumulate a second copy of it.

import (
	"fmt"
	"strings"

	gmai "go-micro.dev/v6/ai"
)

// historyBudget is how much conversation one question may carry, in characters.
//
// A bound on size rather than on turns, because size is what is actually being
// spent and a turn is not a unit of anything — one is "yes", the next is a
// forty-line answer with a table in it. Counting turns meant a cap low enough
// to survive the worst turn, applied to all of them, which is how six became
// the number.
//
// 48,000 characters is roughly 12,000 tokens: a small fraction of any current
// model's window, and far more than the tool definitions and system prompt take
// up beside it. It is deliberately generous — the failure being avoided is a
// conversation somebody has been adding to for a month, not an ordinary one.
//
// What this does not yet do is compact. When the budget is reached the oldest
// turns are dropped, and dropping the oldest can lose the thing that set the
// task. Summarising them instead is the next step and is what Claude Code and
// Shelley both do; it costs a model call, which is why it is a separate change
// rather than smuggled in here.
const historyBudget = 48_000

// threadMemory is a conversation handed to a go-micro agent for one question.
type threadMemory struct {
	msgs []gmai.Message
	// dropped is how many turns did not fit, for the note that says so.
	dropped int
}

// history turns Mu's record of a conversation into the messages a model is
// given: newest first until the budget is spent, then back into order.
//
// Newest first because the recent turns are the ones the question is most
// likely about, and a conversation that will not fit should lose its beginning
// rather than its end.
//
// Roles are normalised to the two a provider understands. Mu writes "user" and
// "assistant" already; anything else would become a role no provider accepts,
// and a message from an unknown speaker is better read as the user's than
// dropped.
// briefing is what is true right now, as one block, or "" when there is nothing
// to say.
//
// Separate from the instructions on purpose. See the note on sys in
// buildNativeAgent: a provider caches the prefix up to a breakpoint at the end
// of the system prompt, which is where the tool catalogue is cached too, and a
// system prompt carrying a timestamp is a prefix that never repeats. So the
// clock, the account's own context and what has just happened come in here
// instead, behind the breakpoint, where changing them costs nothing.
func briefing(parts []string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n\n")
}

// history is the conversation the model is given: what is true now, then what
// was said before.
//
// The briefing goes first and as the user's, because it is what the person
// asking would have told it if they had had to. A provider takes two roles and
// the alternative is to put facts in the assistant's mouth, which is a model
// reading its own words as something it already said.
func history(brief string, turns []QueryMessage) *threadMemory {
	m := &threadMemory{}

	kept := make([]gmai.Message, 0, len(turns))
	spent := 0
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		if t.Text == "" {
			continue
		}
		// Whole turns only. Half an answer is worse than no answer — it reads
		// as something the model said and stops mid-sentence — which is the
		// mistake the 300-character truncation was making on every turn.
		if spent+len(t.Text) > historyBudget && len(kept) > 0 {
			m.dropped = i + 1
			break
		}
		spent += len(t.Text)
		role := "user"
		if t.Role == "assistant" {
			role = "assistant"
		}
		kept = append(kept, gmai.Message{Role: role, Content: t.Text})
	}

	for i := len(kept) - 1; i >= 0; i-- {
		m.msgs = append(m.msgs, kept[i])
	}

	// What the dropped turns were about, in front of what is left.
	//
	// The beginning is where somebody says what they are trying to do, so a
	// long working conversation that simply drops its oldest turns forgets its
	// own purpose while remembering the last twenty exchanges of detail. See
	// compact.go.
	//
	// The note is the fallback rather than the answer: a model told that
	// something is missing can only say so, which is honest and no use.
	if m.dropped > 0 {
		opening := summarise(turns[:m.dropped])
		if opening == "" {
			opening = fmt.Sprintf("[%d earlier messages in this conversation are not "+
				"shown. If the answer depends on them, say so rather than guessing.]",
				m.dropped)
		}
		m.msgs = append([]gmai.Message{{Role: "user", Content: opening}}, m.msgs...)
	}

	// In front of all of it, including the note about what was dropped: what is
	// true now is true of the whole conversation, not of its last turn.
	if brief != "" {
		m.msgs = append([]gmai.Message{{Role: "user", Content: brief}}, m.msgs...)
	}
	return m
}

// Add is deliberately nothing. See the package comment: the record is
// internal/thread, and this agent is discarded after one question.
func (m *threadMemory) Add(role, content string) {}

// Messages is the conversation so far, oldest first.
func (m *threadMemory) Messages() []gmai.Message {
	if m == nil {
		return nil
	}
	return m.msgs
}

// Clear is nothing, for the same reason Add is.
func (m *threadMemory) Clear() {}
