package agent

// Recording a run that nobody was watching.
//
// A flow is written by the two HTTP handlers — the page at /agent and the run
// endpoint — and by nothing else. That was fine while every run started with
// somebody pressing a key, and stopped being fine the moment mail could wake an
// agent: a stranger's email could run your agent, spend your credits and send a
// reply signed with your name, and the only trace was a line in an admin log
// with no body on it, capped at five hundred entries. The owner had no way to
// find out what had been said on their behalf.
//
// So a run that nothing was watching writes itself down, in the same place the
// watched ones go, and /agent/runs shows all of them.

import (
	"strings"
	"time"

	"mu/internal/app"
)

// Where a run came from. The default is the page, which is what every flow
// written before this was.
const (
	FromWeb      = ""
	FromMail     = "mail"
	FromSchedule = "schedule"
)

// Recorded describes a run that has already happened.
type Recorded struct {
	Account string
	// Agent is the user-defined agent that answered, or "" for the default.
	Agent string
	// Source is one of the From constants. What triggered this.
	Source string
	// Trigger names who or what set it off, in words — "email from
	// asim@aslam.me". Shown so the owner can tell an errand they asked for
	// from one somebody else asked for.
	Trigger string
	Prompt  string
	Answer  string
	// Err is set when the run failed, or when the answer could not be
	// delivered. A reply that was composed and never sent is not a success,
	// and the record should not imply the person received it.
	Err error
	// Started is when the run began, so the record shows the real duration
	// rather than the moment it finished.
	Started time.Time
}

// Record writes a completed run to the same store the pages read.
//
// Returns the flow id, so a caller that later learns the reply bounced can say
// so against the run it belongs to.
func Record(r Recorded) string {
	if strings.TrimSpace(r.Account) == "" {
		return ""
	}
	started := r.Started
	if started.IsZero() {
		started = time.Now()
	}

	f := &Flow{
		ID:        newFlowID(),
		AccountID: r.Account,
		Agent:     r.Agent,
		Source:    r.Source,
		Trigger:   r.Trigger,
		Prompt:    r.Prompt,
		Answer:    r.Answer,
		Status:    "done",
		CreatedAt: started,
	}
	if r.Err != nil {
		f.Status = "error"
		f.Error = r.Err.Error()
	}
	// The rendered form is what the runs page shows. Doing it here means a
	// recorded run reads the same as a watched one rather than as raw text.
	if f.Answer != "" {
		f.HTML = `<div class="card" id="agent-response">` + app.RenderString(f.Answer) + `</div>`
	}

	if err := saveFlow(f); err != nil {
		return ""
	}
	return f.ID
}

// Failed marks an already-recorded run as having gone wrong after the fact.
//
// The case this exists for: the agent answered, the answer was recorded, and
// then the mail server could not deliver it. Without this the run says done and
// the person never got their reply.
func Failed(id string, err error) {
	if id == "" || err == nil {
		return
	}
	updateFlow(id, func(f *Flow) {
		f.Status = "error"
		f.Error = err.Error()
	})
}
