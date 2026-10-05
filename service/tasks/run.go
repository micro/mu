package tasks

// Assignments publish durable events. Workers claim an attempt and report
// progress and outcomes through this service; tasks never invoke an agent.

import (
	"fmt"
	"github.com/google/uuid"
	"sync"
)

// Kind is what a task is called on the bus, so a subscriber can tell a task
// from a standing instruction and put the answer in the right place.
const Kind = "task"

// runMu serializes the read-and-transition in Run. The durable status rejects
// later starts; the mutex prevents simultaneous callers both observing todo.
// It is held only while claiming and publishing, never during agent execution.
var runMu sync.Mutex

// Run hands a task to the agent and returns immediately.
//
// It returns immediately because an agent run takes seconds to a minute, and a
// request held open for that is a page that looks broken. The task moves to
// "doing" now, then to "done" or "failed" with its result when the work
// finishes, so the list is the progress indicator.
func Run(owner, id string) error {
	runMu.Lock()
	defer runMu.Unlock()
	if claims[owner+":"+id] != nil {
		return fmt.Errorf("the previous worker is still stopping")
	}
	t, err := Get(owner, id)
	if err != nil {
		return err
	}
	if t.Occurrence != nil {
		return fmt.Errorf("manage this execution through its schedule")
	}
	if t.Delivery != nil {
		return fmt.Errorf("the previous result is still being delivered")
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
	// Preserve legacy activity before the first run under the new recorder.
	if len(t.Attempts) == 0 && (len(t.Steps) > 0 || t.Result != "") {
		t.Attempts = append(t.Attempts, Attempt{ID: uuid.NewString(), Status: t.Status, Finished: t.Updated, Steps: t.Steps, Report: t.Result})
	}
	t.Attempts = append(t.Attempts, Attempt{ID: uuid.NewString(), Started: now(), Status: StatusDoing})
	if _, err := update(owner, t.ID, "", "", StatusDoing, Agent, "", map[string]any{"attempts": encodeAttempts(t.Attempts), "result": nil, "archived": false}, []Step{}); err != nil {
		return err
	}

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
