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
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"mu/agent"
	"mu/inbox"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/origin"
	"mu/internal/thread"
	"mu/internal/version"
	"mu/service/events"
	"mu/service/mail"
	"mu/service/tasks"
)

// Load subscribes to the work agents are asked to do.
func Load() {
	loadAppBuilds()
	go retryDeliveries()
	for _, topic := range []string{event.TaskStarted, event.ScheduleDue} {
		go func(topic string) {
			for {
				err := event.Consume(context.Background(), "work-"+strings.ReplaceAll(topic, ".", "-"), []string{topic}, consumeWork)
				app.Log("work", "event consumer stopped: %v", err)
				time.Sleep(5 * time.Second)
			}
		}(topic)
	}
}

// request is one piece of work, off the bus.
type request struct {
	EventID string
	Account string
	Kind    string
	ID      string
	Title   string
	Prompt  string
	// Thread is the conversation it came out of, empty when nobody asked for
	// it in one. See answered.
	Thread string
	// Agent is which agent was given this, empty for whichever one this
	// instance runs by default. See run, which is where a name becomes an
	// instruction and a tool scope.
	Agent string
}

// run does the work and puts the answer where it belongs.
func run(r request) {
	if benefitRun(r) {
		return
	}
	runWithQuery(r, agent.QueryWithOpts)
}

func runWithQuery(r request, query func(string, string, agent.QueryOpts) (string, error)) {
	ctx, cancel := context.WithCancel(context.Background())
	key := r.Account + ":" + r.ID
	if _, loaded := activeRuns.LoadOrStore(key, cancel); loaded {
		cancel()
		return
	}
	defer activeRuns.Delete(key)
	defer cancel()
	runID := ""

	if r.Kind == tasks.Kind {
		t, err := tasks.Get(r.Account, r.ID)
		if err != nil {
			app.Log("work", "task %s is unavailable: %v", r.ID, err)
			return
		}
		if t.Delivery != nil {
			if err := deliverTask(t); err != nil {
				app.Log("work", "task %s result delivery pending: %v", r.ID, err)
			}
			return
		}
		if t.Status == tasks.StatusCanceled || t.Archived || t.Status == tasks.StatusDone || t.Status == tasks.StatusFailed || t.Status == tasks.StatusBlocked {
			return
		}
		if len(t.Attempts) > 0 {
			runID = t.Attempts[len(t.Attempts)-1].ID
		}
	}

	var steps []tasks.Step
	var stepsMu sync.Mutex
	defer func() {
		if rec := recover(); rec != nil {
			failure := fmt.Errorf("the agent run stopped unexpectedly")
			app.Log("work", "running %s %s panicked: %v", r.Kind, r.ID, rec)
			if r.Kind == tasks.Kind {
				stepsMu.Lock()
				partial := append([]tasks.Step(nil), steps...)
				stepsMu.Unlock()
				if runID != "" {
					saveProgress(r.Account, r.ID, runID, partial, "", diagnosticText(fmt.Sprint(rec)), version.String())
				}
				finishTask(r, "", partial, failure)
			} else {
				answered(r, "", failure)
			}
		}
	}()

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
	// Which agent, and what it was told.
	//
	// Work ran as the default agent whatever it was given to: no agent
	// travelled on the bus, so the one place work is actually handed over
	// answered every conversation with the general instruction and every tool
	// on the box. That made having more than one agent pointless exactly where
	// it should have mattered most.
	//
	// A deleted or unavailable specialist cannot be replaced by a broader
	// default. Resolve again when queued work starts, and refuse if missing.
	var opts agent.QueryOpts
	if r.Agent != "" {
		if plat := agent.Platform(r.Agent); plat != nil {
			opts = agent.PlatformOpts(plat)
		} else if o, err := agent.AskAs(r.Account, r.Agent); err == nil {
			opts = o
		} else {
			if r.Kind == tasks.Kind {
				finishTask(r, "", nil, &blockedOutcome{summary: err.Error()})
			} else {
				deliver(r, "", err)
				answered(r, "", err)
			}
			return
		}
	}

	// Work that came out of a conversation is framed as acting on one, and the
	// framing goes in front of whatever the agent was already told — which is
	// what InboxPrompt's argument is for. The other way round, a specialist's
	// standing instruction would be read as the thing to act on.
	system := opts.System
	if r.Thread != "" {
		system = agent.InboxPrompt(opts.System)
	}

	opts.RunContext = ctx
	opts.System = system
	if r.Kind == tasks.Kind {
		opts.RawReply = true
		opts.OutputInstruction = outcomeInstruction
	}
	opts.OnStepStart = func(s agent.Step) {
		stepsMu.Lock()
		defer stepsMu.Unlock()
		steps = append(steps, recordedStep(s, "running"))
		if runID != "" {
			saveProgress(r.Account, r.ID, runID, steps, "", "", version.String())
		}
	}
	opts.OnStep = func(s agent.Step) {
		stepsMu.Lock()
		defer stepsMu.Unlock()
		status := "done"
		if !s.OK {
			status = "failed"
		}
		step := recordedStep(s, status)
		replaced := false
		for i := range steps {
			if s.ID != "" && steps[i].ID == s.ID {
				steps[i] = step
				replaced = true
				break
			}
		}
		if !replaced {
			steps = append(steps, step)
		}
		if runID != "" {
			saveProgress(r.Account, r.ID, runID, steps, "", "", version.String())
		}
	}
	answer, err := query(r.Account, workPrompt(r), opts)
	stepsMu.Lock()
	completedSteps := append([]tasks.Step(nil), steps...)
	stepsMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	rawReport := answer

	switch r.Kind {
	case tasks.Kind:
		if err == nil {
			answer, err = readOutcome(answer)
		}
		failure := ""
		if err != nil {
			failure = err.Error()
		}
		if runID != "" {
			saveProgress(r.Account, r.ID, runID, completedSteps, diagnosticText(rawReport), diagnosticText(failure), version.String())
		}
		finishTask(r, answer, completedSteps, err)
		return
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

// The text of a conversation does not identify its delivery address. Give the
// worker the owned thread's destination so an explicit request to reply can
// actually send, while a summary or draft remains a private result.
func workPrompt(r request) string {
	var context strings.Builder
	if r.Kind == tasks.Kind {
		fmt.Fprintf(&context, "You are already executing task %q. Do the requested work now; do not create or reassign another task for this same work. Verify the requested outcome and return the structured report required by your instructions; the runner records its status. Do not claim completion from inspection or successful tool transport alone. Read shell exit codes; missing interpreters are not successful edits. Use available tools such as shell Write rather than repeatedly invoking unavailable programs.\n\n", r.ID)
	}
	if th := thread.Get(r.Account, r.Thread); th != nil && th.Client == thread.ChatClient && !strings.HasPrefix(th.Key, "xmpp_") {
		fmt.Fprintf(&context, "The source conversation is chat room %q. If the owner asks you to send or reply in this conversation, use the chat Send tool with that exact room id. A draft or summary is not a request to send. Never claim a reply was sent unless the tool confirms it; if the tool is unavailable, say so. Your final answer is a private report to the owner.\n\n", th.Key)
	}
	return context.String() + r.Prompt
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
		text = "That did not work: " + ai.FailureMessage(err)
	}
	from := r.Agent
	if from == "" {
		from = agent.DefaultName()
	}
	agent.AnsweredAs(r.Account, r.Thread, text, "", from)
}

// finishTask writes the result back onto the task.
//
// A failed run remains open with an explicit failed state. It is not picked
// up automatically; the owner can review its steps before retrying.
func finishTask(r request, answer string, steps []tasks.Step, err error) {
	status, result := tasks.StatusDone, strings.TrimSpace(answer)
	if result == "" && err == nil {
		err = fmt.Errorf("the agent returned no outcome")
	}
	reply := result
	if err != nil {
		app.Log("work", "task %q failed for %s: %v", r.Title, r.Account, err)
		status = tasks.StatusFailed
		result = "Last run failed: " + ai.FailureMessage(err)
		reply = "That did not work: " + ai.FailureMessage(err)
	}
	var blocked *blockedOutcome
	if errors.As(err, &blocked) {
		status, result, reply = tasks.StatusBlocked, blocked.summary, blocked.summary
	}
	from := r.Agent
	if from == "" {
		from = agent.DefaultName()
	}
	reply += "\n\n[View work](" + origin.Self() + "/work?id=" + url.QueryEscape(r.ID) + ")"
	t, saveErr := tasks.RecordOutcome(r.Account, r.ID, status, result, reply, from, steps)
	if saveErr != nil {
		app.Log("work", "saving task outcome %s: %v", r.ID, saveErr)
		return
	}
	if err := deliverTask(t); err != nil {
		app.Log("work", "task %s result delivery pending: %v", r.ID, err)
	}
}

// deliverTask never invokes the agent. The stable reference makes retrying
// after the message was written but before acknowledgement safe.
func deliverTask(t *tasks.Task) error {
	if t == nil || t.Delivery == nil {
		return nil
	}
	d := t.Delivery
	if err := agent.AnsweredOnce(t.Owner, t.Thread, d.Text, d.From, "task-result:"+d.ID); err != nil {
		return err
	}
	return tasks.AcknowledgeDelivery(t.Owner, t.ID, d.ID)
}

func retryDeliveries() {
	for {
		for _, acc := range auth.AllAccounts() {
			if acc == nil {
				continue
			}
			after := ""
			for {
				batch := tasks.PendingDeliveries(acc.ID, after)
				if len(batch) == 0 {
					break
				}
				for _, t := range batch {
					after = t.Delivery.ID
					if err := deliverTask(t); err != nil {
						app.Log("work", "task %s result delivery pending: %v", t.ID, err)
					}
				}
			}
		}
		time.Sleep(time.Minute)
	}
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
		body = "This scheduled task failed: " + ai.FailureMessage(err)
	}
	if body == "" {
		return
	}
	acc, accErr := auth.GetAccount(r.Account)
	if accErr != nil {
		return
	}
	tag := "scheduled"
	isBrief := false
	if e := scheduledBrief(r); e != nil {
		isBrief = true
		tag = "brief"
		body += "\n\n---\n[Manage your brief and plan](" + origin.Self() + "/events?view=brief)."
	}
	messageID := "<" + uuid.NewString() + "@" + mail.ConfiguredDomain() + ">"
	if r.EventID != "" {
		messageID = "<schedule-" + r.EventID + "@" + mail.ConfiguredDomain() + ">"
	}
	sender := agent.NameOf(r.Account, r.Agent)
	if sender == "" {
		sender = agent.DefaultName()
	}
	if sender == "" {
		sender = "Micro"
	}
	delivery := mail.Delivery{
		From: sender, FromID: "agent@" + mail.ConfiguredDomain(),
		To: acc.Name, ToID: acc.ID, Tag: tag,
		Subject: r.Title, Body: body, MessageID: messageID,
	}
	if sendErr := mail.SendMessageTo(delivery); sendErr != nil {
		app.Log("work", "delivering scheduled result for %s: %v", r.Account, sendErr)
		return
	}
	if isBrief && err == nil {
		link := inbox.MailURL(mail.InboundMail{Owner: acc.ID, From: delivery.FromID, FromName: delivery.From, To: acc.ID + "+" + tag + "@" + mail.ConfiguredDomain(), Subject: r.Title, Body: delivery.Body, MessageID: messageID, Tag: tag})
		event.Announce("brief", strings.TrimSpace(answer), link, r.Account)
	}
}

var activeRuns sync.Map // account:task -> context.CancelFunc
