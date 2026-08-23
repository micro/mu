// Package work is what the agent does when somebody asks for a piece of work.
//
// A task assigned and a standing instruction falling due are the same fact: an
// account has asked an agent to do something while nobody is watching. Both
// used to run the agent from inside the service that held the record — a
// RunAgent function variable main() filled in, in service/tasks and again in
// service/events — because a service may not import an agent and a hook is how
// that compiles anyway.
//
// The rule has a reason rather than a convention behind it: a service answers a
// question about state, an agent decides which question to ask, and a service
// calling an agent is asking the model what its own answer should be.
//
// So the services announce and this subscribes, which is what service/mail
// already did and agent/mail is the worked example of. The direction is now the
// one the layering asks for: an agent may import a service, and both of the
// services below are imported here plainly.
//
// # Where the answer goes
//
// That is this package's knowledge and deliberately not the service's. A task
// keeps its result, because a task is a record you come back to. A standing
// instruction is mailed, because it ran while you were elsewhere and an answer
// on a page you are not looking at is an answer nobody gets.
//
// And where the work came out of a conversation, the outcome goes back to it as
// well as to the record it belongs to. Work does not usually start on a page:
// somebody writes in, it turns out to take an hour, and a result on a task page
// is a result nobody reads — they asked in a thread and that is where they are
// looking. Both, not either: the task is what the work *is*, the thread is
// where it was asked for.
//
// # It always says something
//
// service/events had a rule worth keeping: a run that could not happen is news
// the owner needs, because "a standing instruction that goes quiet looks like
// the instruction was forgotten, and they would have no way to tell the
// difference." It could only half honour that, since it did not know why a run
// failed until the hook returned. This does know, so a failure is delivered
// like an answer.
package work

import (
	"strings"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/service/events"
	"mu/service/mail"
	"mu/service/tasks"
)

// Load subscribes to the work agents are asked to do.
func Load() {
	sub := event.Subscribe(event.WorkForAgent)
	go func() {
		for e := range sub.Chan {
			r, ok := requestFrom(e.Data)
			if !ok {
				app.Log("work", "a work request carried no account or prompt")
				continue
			}
			// One goroutine per request: a run takes seconds to a minute, and
			// the scheduler fires everything due in the same pass. One slow
			// briefing must not hold up the rest.
			go func(r request) {
				defer func() {
					if rec := recover(); rec != nil {
						app.Log("work", "running %s %s panicked: %v", r.Kind, r.ID, rec)
					}
				}()
				run(r)
			}(r)
		}
	}()
}

// request is one piece of work, off the bus.
type request struct {
	Account string
	Kind    string
	ID      string
	Title   string
	Prompt  string
	// Thread is the conversation it came out of, empty when nobody asked for
	// it in one. See answered.
	Thread string
}

// requestFrom reads a work request, and reports false when there is nothing to
// run — which is what a subscriber on the wrong topic gets.
func requestFrom(data map[string]interface{}) (request, bool) {
	str := func(k string) string { s, _ := data[k].(string); return s }
	r := request{
		Account: str("account"), Kind: str("kind"), ID: str("id"),
		Title: str("title"), Prompt: str("prompt"), Thread: str("thread"),
	}
	return r, r.Account != "" && r.Prompt != ""
}

// run does the work and puts the answer where it belongs.
func run(r request) {
	var steps []tasks.Step

	// Work that came out of a conversation is framed as acting on one.
	//
	// The distinction the framing carries is who the words belong to: the thread
	// is what arrived, and the instruction is the owner standing over it saying
	// what to do about it. Without it the specific failure is that the agent
	// reads "add that to my calendar", takes it for a message from whoever sent
	// the email, and replies to them about calendars — see agent.InboxPrompt.
	//
	// It used to be applied by the inbox, on a control that ran the agent inside
	// the POST and made you wait for it. That control is gone and the framing is
	// not: handing a conversation over is the same act, done properly, so it
	// belongs here where the handing-over is run.
	system := ""
	if r.Thread != "" {
		system = agent.InboxPrompt("")
	}

	answer, err := agent.QueryWithOpts(r.Account, r.Prompt, agent.QueryOpts{
		System: system,
		OnStep: func(s agent.Step) {
			steps = append(steps, tasks.Step{
				Tool:    s.Tool,
				Detail:  tasks.StepDetail(s.Args),
				OK:      s.OK,
				Seconds: s.Took.Seconds(),
			})
		},
	})

	switch r.Kind {
	case tasks.Kind:
		finishTask(r, answer, steps, err)
	case events.Kind:
		deliver(r, answer, err)
	default:
		app.Log("work", "nothing knows what to do with a %q", r.Kind)
	}

	// And back to whoever asked, where they asked in a conversation.
	//
	// Work does not usually start on a page. Somebody writes in, it turns out
	// to take an hour, and a result sitting on a task page is a result nobody
	// reads — they asked in a thread and that is where they are looking. The
	// record is the same one every client renders, so this lands in /inbox
	// beside what they wrote.
	answered(r, answer, err)
}

// answered puts the outcome back on the conversation the work came out of.
//
// Through agent.Answered, which is what every client uses to write down what an
// agent said — there is no version of this special enough to reach past it.
// Silent when there was no conversation: a task somebody wrote on the page and
// a schedule falling due have nowhere to go back to, and that is not a failure.
func answered(r request, answer string, err error) {
	if r.Thread == "" {
		return
	}
	text := strings.TrimSpace(answer)
	if err != nil {
		// Said rather than swallowed, for the reason in the package comment:
		// silence is indistinguishable from work nobody picked up.
		text = "That did not work: " + err.Error()
	}
	agent.Answered(r.Account, r.Thread, text, "")
}

// finishTask writes the result back onto the task.
//
// A failed run leaves the task open: work that failed is work still to do, not
// work finished badly, and the reason belongs where somebody will see it.
func finishTask(r request, answer string, steps []tasks.Step, err error) {
	if err != nil {
		app.Log("work", "task %q failed for %s: %v", r.Title, r.Account, err)
		tasks.Update(r.Account, r.ID, "", "", tasks.StatusTodo, "", //nolint:errcheck
			"Last run failed: "+err.Error(), steps)
		return
	}
	tasks.Update(r.Account, r.ID, "", "", tasks.StatusDone, "", //nolint:errcheck
		strings.TrimSpace(answer), steps)
}

// deliver mails what a standing instruction produced.
//
// Mail is the delivery that survives being away from the screen, and this
// instance runs the inbox. Tagged so an agent can read back only its own
// scheduled results.
//
// A failure is delivered too, for the reason in the package comment: silence is
// indistinguishable from an instruction nobody kept.
func deliver(r request, answer string, err error) {
	body := strings.TrimSpace(answer)
	if err != nil {
		app.Log("work", "standing instruction %q failed for %s: %v", r.Title, r.Account, err)
		body = "This scheduled task failed: " + err.Error()
	}
	if body == "" {
		return
	}
	acc, accErr := auth.GetAccount(r.Account)
	if accErr != nil {
		return
	}
	mail.SendMessageTo(mail.Delivery{ //nolint:errcheck
		From: "Mu", FromID: "agent@" + mail.ConfiguredDomain(),
		To: acc.Name, ToID: acc.ID, Tag: "scheduled",
		Subject: r.Title, Body: body,
	})
}
