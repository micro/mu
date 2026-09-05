package agent

// The answer that landed while you were not looking.
//
// Reported as: asked the agent to build something, watched it say "working",
// refreshed the page, and never got an update again.
//
// Nothing was lost. A run drives itself off the request that started it but
// does not depend on it — the goroutine finishes, Answered writes the reply
// into the record and updateFlow marks the run done, whether or not anybody is
// still holding the socket the tokens were going to. What was lost is the
// browser: the SSE reader dies with the page, and the reloaded page renders the
// conversation as it stands *at that moment*, which is your question and
// nothing after it. Then it sits there, because a page that has finished
// loading has no reason to look again.
//
// So it looks again. Two pieces: the render says whether this conversation is
// waiting on an answer, and this endpoint says whether one has arrived.
//
// # Why not resume the stream
//
// Because the stream is not the work. Reattaching an SSE reader to a run in
// progress means the run has to hold its output somewhere replayable and know
// that a second reader may turn up — a buffer per run, with a lifetime, for the
// sake of showing tokens appearing. What somebody who has refreshed actually
// wants is the answer. Polling for it is a handler with no state in it, and it
// works for the runs that were never streaming to a browser at all: a reply
// coming back to a question that arrived by mail lands in the same record and
// appears on this page the same way.

import (
	"net/http"
	"strings"

	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// Pending reports whether a conversation is waiting on an answer.
//
// The last thing said is the person's. Every path that answers — the web, mail,
// a schedule — writes the question with Said before the run starts and the
// reply with Answered when it ends, so a conversation whose newest message is
// not the agent's is one where a reply is expected and has not arrived.
//
// It cannot tell "still running" from "died with the process", and does not try.
// Both are the same to a reader: no answer yet. What separates them is time,
// which is the client's to judge — see the give-up in the poll.
func Pending(accountID, threadID string) bool {
	if accountID == "" || threadID == "" {
		return false
	}
	msgs := thread.Messages(accountID, threadID, 1)
	return len(msgs) == 1 && msgs[0].Role != thread.RoleAgent
}

// PendingHandler serves GET /agent/pending?thread=<id>.
//
// Answers with what the page does not have: the turns after the last thing the
// person said, which is the answer if one has landed and nothing at all if it
// has not.
//
// Defined that way rather than as "everything after message N" on purpose.
// thread.Messages returns the last N of a conversation, so an index into it
// shifts by one every time a message is added — index arithmetic between two
// requests would skip a turn on any conversation long enough to fill the
// window. "After the last thing you said" is a position in the conversation
// rather than in a slice, and it does not move.
func PendingHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	id := strings.TrimSpace(r.URL.Query().Get("thread"))
	if id == "" {
		app.RespondJSON(w, map[string]any{"waiting": false})
		return
	}

	msgs := thread.Messages(acc.ID, id, inbox.MessagesShown)
	if len(msgs) == 0 {
		app.RespondJSON(w, map[string]any{"waiting": false})
		return
	}

	// Everything after the last thing the person said.
	cut := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != thread.RoleAgent {
			cut = i
			break
		}
	}

	// Who is answering in this conversation. The thread records it — see
	// thread.Thread.Agent — so the poll and the page agree without either
	// guessing from the message.
	who := ""
	if th := thread.Get(acc.ID, id); th != nil {
		who = agentTitle(acc.ID, th.Agent)
	}
	var b strings.Builder
	for _, m := range msgs[cut+1:] {
		b.WriteString(renderTurn(m, who))
	}
	steps := flowProgress(acc.ID, id)
	type progressStep struct {
		Label  string `json:"label"`
		Status string `json:"status"`
	}
	progress := make([]progressStep, 0, len(steps))
	for _, step := range steps {
		label := step.Label
		if label == "" {
			label = step.Tool
		}
		if label != "" {
			progress = append(progress, progressStep{Label: label, Status: step.Status})
		}
	}
	waiting := msgs[len(msgs)-1].Role != thread.RoleAgent
	runError := ""
	if waiting {
		runError = flowError(acc.ID, id)
		waiting = runError == ""
	}

	app.RespondJSON(w, map[string]any{
		"waiting": waiting,
		"html":    b.String(),
		"steps":   progress,
		"error":   runError,
	})
}

// renderTurn is one message as the transcript draws it.
//
// Shared with renderThreadTurns so a turn appended by the poll is the same
// markup as one rendered with the page. Two renderers for the same thing is
// how a reloaded conversation ends up styled differently below the fold.
//
// by is the name of whoever wrote it, and an answer carries it.
//
// Nothing did. An agent's reply was a bare card, so on any screen where more
// than one of them can appear — the inbox, Home, a conversation you reopened
// from /recall — nothing said which agent had answered, and platform-written
// content in a card of the same shape read as an agent's too. Mu is the place
// and the agent is who answers; the second half of that was never on screen.
//
// Empty leaves it off, which is what a caller with no thread to ask has. A
// wrong name is worse than none: that is the whole property being fixed.
func renderTurn(m thread.Message, by string) string {
	if m.Role == thread.RoleAgent {
		return `<div class="mu-agent">` + byline(by) + `<div class="card">` +
			app.RenderString(m.Text) + `</div></div>`
	}
	return `<div class="mu-user">` + htmlEsc(m.Text) + `</div>`
}

// byline is the name above an answer, in the same shape the live one takes —
// see mu-by in internal/app/chat.go, which builds it in the browser for a reply
// arriving now. Two renderers for one thing again, and unavoidable: one of them
// runs before the answer exists.
func byline(who string) string {
	if strings.TrimSpace(who) == "" {
		return ""
	}
	return `<div class="mu-by">` + htmlEsc(who) + `</div>`
}
