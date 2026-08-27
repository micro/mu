package tasks

// Handing a task to the agent.
//
// A list the agent can read is a to-do app. What makes this a service is that
// the work can be given away: mark a task for the agent, and it runs, and the
// answer comes back onto the task where it can be read later.
//
// This used to run it. There was a RunAgent function variable that main()
// filled in, a goroutine here that called it, and the result written back when
// it returned — a service running an agent, which is the direction the layering
// forbids and the hook is how it compiled. The reason for the rule is not
// tidiness: a service answers a question about state, an agent decides which
// question to ask, and a service calling an agent is asking the model what its
// own answer should be.
//
// So it says what happened instead. The task moves to "doing" and the fact goes
// on the bus; agent/work is subscribed, runs it, and writes the result back
// through Update like any other caller. Nothing here knows an agent exists.
//
// That is the same inversion service/mail made — see internal/event, where mail
// arriving is a fact rather than a call — and it is why assigning a task, mail
// arriving, a schedule firing and somebody typing are one concept with four
// doors rather than four mechanisms.

import (
	"fmt"

	"mu/internal/event"
)

// Kind is what a task is called on the bus, so a subscriber can tell a task
// from a standing instruction and put the answer in the right place.
const Kind = "task"

// Run hands a task to the agent and returns immediately.
//
// It returns immediately because an agent run takes seconds to a minute, and a
// request held open for that is a page that looks broken. The task moves to
// "doing" now and to "done" with its result when the work finishes, so the list
// is the progress indicator.
func Run(owner, id string) error {
	t, err := Get(owner, id)
	if err != nil {
		return err
	}
	if t.Status == StatusDone {
		return fmt.Errorf("that task is already done")
	}
	if t.Status == StatusDoing {
		return fmt.Errorf("the agent is already working on that")
	}

	// Marked before it is announced, so the guard above holds against a second
	// press: the status is the lock, and it is the one a reader can see. There
	// was an in-memory map beside it doing the same job, which meant two
	// answers to "is this running" and only one of them survived a restart —
	// a task left "doing" by a crash could never be run again.
	if _, err := Update(owner, t.ID, "", "", StatusDoing, Agent, ""); err != nil {
		return err
	}

	event.RequestWork(t.Owner, Kind, t.ID, t.Title, prompt(*t), t.Thread, t.Agent)
	return nil
}

// Running reports whether the agent is working on a task right now.
func Running(t *Task) bool { return t != nil && t.Status == StatusDoing }

// prompt is what the agent is actually asked. The title is the instruction and
// the detail is the context; saying so beats hoping the model infers it.
func prompt(t Task) string {
	if t.Detail == "" {
		return t.Title
	}
	return t.Title + "\n\n" + t.Detail
}
