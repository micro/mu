package agent

// How a run is framed for the client it answers, and where it came from.
//
// What was said is internal/thread. This is what is left once that moved out:
// the mail framing, and the note on a workflow record saying which client asked
// for it.

import (
	"strings"
)

// Via is where a run came from: which client, and which conversation on it.
//
// Metadata on the workflow record, so the runs page can say where an answer was
// asked for. Finding a conversation again is internal/thread's job — this held
// mail's message ids for a while, which made a workflow record the place a
// reply looked for its history.
type Via struct {
	// Client is discord, telegram, whatsapp, mail, web, cli.
	Client string
	// Thread is the conversation in the record, where there is one.
	Thread string
	// From is who set it off, where that is not the account itself: somebody
	// else's address, on mail answered on the owner's behalf.
	From string
}

// MailPrompt frames a run as answering an email, on top of whatever system
// prompt the agent already has.
//
// Without it the agent behaves as it does on the page, and the page is a place
// where a follow-up costs a second. Asked something by mail it offered to fetch
// the answer, asked which of two things was wanted, and — given a subject line
// that mentioned mail — drafted a reply for approval and addressed it to
// itself. Every one of those is a sensible move in a chat window and a dead end
// in an inbox, where the next turn is hours away and may never come.
//
// Prepended rather than replacing, so a specialist keeps its own instructions:
// this says how to answer, not what the agent is.
func MailPrompt(base string) string {
	out := mailFraming
	if base = strings.TrimSpace(base); base != "" {
		out += "\n\n" + base
	}
	return out
}

const mailFraming = `You are answering an email. What you write is sent to the
person as your reply — there is no draft step, no approval, and nobody edits it
after you.

Answer the message. Do the work now:

- Never offer to do something and wait to be told yes. If a reasonable next step
  is obvious, take it and report what you found.
- Never say you do not have information without first using your tools to get
  it. You can look things up; that is what you are for.
- Ask a question only when you genuinely cannot proceed without one, and answer
  as much as you can alongside it. A question costs a round trip measured in
  hours, so a half answer now beats a whole one tomorrow.
- Do not summarise their inbox, unread mail or account unless they asked about
  it. They wrote to you about something specific.
- Do not draft an email for them and do not address anything to yourself. You
  are the correspondent, not an assistant helping somebody write mail.

Write as you would to a colleague: plain prose, no preamble about what you are
about to do, and no sign-off — the message already says who it is from.`
