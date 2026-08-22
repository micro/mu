package events

// Running a standing instruction.
//
// An event with a prompt is not a reminder — it is the agent doing a piece of
// work while nobody is watching, which is the most valuable thing this instance
// does.
//
// This used to run it. There was a RunAgent function variable main() filled in,
// and RunPrompt called it and handed the answer back to be delivered — a
// service running an agent, which is the direction the layering forbids and the
// hook is how it compiled.
//
// It says what happened instead. When an instruction comes due the fact goes on
// the bus and agent/work picks it up; nothing here knows an agent exists. Same
// inversion service/mail made, and the same one service/tasks made beside it,
// so a task assigned and a schedule firing are one concept rather than two
// mechanisms.
//
// What was lost and put back: RunPrompt always returned something to deliver,
// because "a standing instruction that goes quiet looks like the instruction
// was forgotten, and they would have no way to tell the difference." That rule
// still holds and now lives with the thing that can honour it — the subscriber
// delivers an answer or a reason, because it is the one that knows which
// happened. The check this file can still make is made here: an instruction
// belonging to no account never reaches the bus.

import (
	"strings"

	"mu/internal/auth"
	"mu/internal/event"
)

// Kind is what a standing instruction is called on the bus, so a subscriber can
// tell one from a task and put the answer in the right place.
const Kind = "event"

// requestWork asks for a fired event's standing instruction to be run.
//
// Nothing is asked for an event with no prompt: that is a reminder, and OnFire
// has already delivered it.
func requestWork(e *Event) {
	prompt := strings.TrimSpace(e.Prompt)
	if prompt == "" {
		return
	}
	// Whose instruction it is, checked because it is a real question and not
	// because of money. It used to fall out of the credit check — an unknown
	// account could not be charged, so it could not run — and that guard left
	// with the charge when running the agent stopped costing credits. A rule
	// that only worked as a side effect of another rule is one nobody was
	// holding.
	if _, err := auth.GetAccount(e.Owner); err != nil {
		return
	}
	event.RequestWork(e.Owner, Kind, e.ID, e.Title, prompt)
}
