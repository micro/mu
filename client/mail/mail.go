package mail

// Mail as a client.
//
// Discord connects to Discord and Telegram to Telegram; here the protocol is
// SMTP and this instance runs the server, which service/mail owns. That does
// not make the two the same thing. service/mail is the capability — an inbox,
// an address, delivery — and this is a client of it: it speaks the shape mail
// arrives in, hands the message to the agent, and turns the answer back into a
// reply. The same job discord does, against a different protocol.
//
// It lived in internal/server/hooks.go, which is the file that exists to hold
// cycles nothing else could avoid. This was never one of those: a client may
// import the agent and the service freely, and only the registration had to
// happen at wiring time. Two hundred lines of deciding which agent answers and
// what it says were sitting in the wiring.

import (
	"fmt"
	"strings"
	"time"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/thread"
	"mu/service/mail"
)

// Client names this client in a run record. See discord.Client.
const Client = "mail"

// historyTurns is how much of an email thread an agent is reminded of.
//
// Six messages is three exchanges, which covers the back-and-forth people
// actually have by mail and stops a thread somebody has been adding to for a
// month from costing more in prompt than the answer is worth.
const historyTurns = 6

// chainKey names the conversation a message belongs to, mail's way.
//
// The root of the reference chain: the first message id in References, or this
// message's own when it starts one. That is what every mail client means by a
// thread, and it is stable for everyone in it.
//
// It used to be the address written to, which made every message anybody ever
// sent to agent@ one conversation — a stranger's question and yours filed
// together, and the agent handed both as history. Ask still resolves a reply by
// its In-Reply-To before this is consulted; this is what a *new* message opens.
func chainKey(m mail.InboundMail) string {
	if refs := thread.Refs(m.References); len(refs) > 0 {
		return refs[0]
	}
	if refs := thread.Refs(m.InReplyTo); len(refs) > 0 {
		return refs[0]
	}
	if m.MessageID != "" {
		return m.MessageID
	}
	// Nothing to key on at all. The address is a poor thread — everyone who
	// writes to it lands in one — so it is scoped by sender, which at least
	// keeps two strangers apart.
	return m.To + " " + m.From
}

// Load registers the agent for mail arriving at an address that names one, and
// the recorder for everything that arrives at all.
//
// Two addresses, one handler: you+research@ names one of the account's own
// agents, agent+news@ names one of this instance's. See agent/platform.go.
//
// The recorder is registered separately and answers a different question — see
// mail.Delivered and record.go.
func Load() {
	mail.Delivered(recordDelivery)
	mail.Inbound(mail.Tagged, answerMail)
	mail.Inbound(mail.AgentMailbox, answerMail)
}

// asked is the message as a question: its subject and its text, in the shape
// the agent is handed and the record keeps.
//
// Prose, not the markup somebody's client happened to send it in. A mail
// composed on a phone arrives as `<div dir="auto">…</div>`, and that was going
// to the agent as the question and into the record as the conversation's name.
// Body is what the inbox renders; Text is what was said.
func asked(m mail.InboundMail) string {
	out := m.Subject
	if b := body(m); b != "" {
		if out != "" {
			out += "\n\n"
		}
		out += b
	}
	return strings.TrimSpace(out)
}

// body is what somebody actually wrote, without the subject in front of it.
//
// asked() is what the agent is handed, where the subject is part of the
// question. The record wants this: the subject belongs to the conversation, and
// putting it at the top of every message means the reader meets it once as the
// heading and again on every message under it. See recordDelivery.
func body(m mail.InboundMail) string {
	if b := strings.TrimSpace(m.Text); b != "" {
		return b
	}
	return strings.TrimSpace(m.Body)
}

// cleanSubject is the subject with the reply and forward markers off the front,
// so a thread is named once and not renamed "Re: Re: Lunch" on the third turn.
//
// thread.Name keeps the first name a conversation is given, so this only
// matters when the first message in is itself a reply — mail forwarded into an
// agent, or a chain this instance joined halfway.
func cleanSubject(s string) string {
	s = strings.TrimSpace(s)
	for {
		l := strings.ToLower(s)
		switch {
		case strings.HasPrefix(l, "re:"):
			s = strings.TrimSpace(s[3:])
		case strings.HasPrefix(l, "fwd:"):
			s = strings.TrimSpace(s[4:])
		case strings.HasPrefix(l, "fw:"):
			s = strings.TrimSpace(s[3:])
		default:
			return s
		}
	}
}

// Mail addressed to an agent wakes it, and it answers in the thread.
//
// Every agent already had an address. Writing to one filed a message in the
// owner's inbox and nothing else — an agent with an address that cannot
// answer is a mailbox with a name on it, and emailing your agent is the
// first thing anyone tries with one.
//
// It answers as that agent: its standing instruction and its scope, so a
// research agent you emailed cannot read your mail unless you gave it mail.
// Charged like any other agent run, checked before the model is asked so a
// run that cannot be paid for does not spend one first — and the sender is
// told, because silence is what this looked like before.
// Who may wake one. The sender has to pass SPF or DKIM and be somebody this
// account knows — its own verified address, checked inside mail, or a name
// in its address book, which is this hook because contacts is a different
// domain and mail should not import it.
// Registered rather than assigned: mail no longer knows what an agent is,
// it knows that something asked for mail at these addresses.
func answerMail(m mail.InboundMail) {
	// Which agent answers, and the two addresses answer it from two
	// different namespaces — see agent/platform.go.
	//
	//   you+research@   your roster. A tag naming nothing is ordinary
	//                   tagged mail and filing it is the whole job.
	//   agent+news@     this instance's own. A name that is not here is a
	//                   typo somebody is waiting on, so it is answered.
	//   agent@          the default, Micro, which is what somebody writing
	//                   for the first time means.
	//
	// The same word means different things on either side and that is the
	// point: your namespace is yours, and a new built-in agent can never
	// take over an address you were already using.
	var (
		a          *agent.Agent // the account's own
		platID     string       // one of this instance's own, by id
		platName   string
		unknownTag string
	)
	switch {
	case m.Shared:
		var ok bool
		if platID, platName, ok = agent.PlatformNamed(m.Tag); !ok {
			unknownTag = m.Tag
		}
	case m.Tag != "":
		if a = agent.ForTag(m.Owner, m.Tag); a == nil {
			return // a tag that is not an agent: ordinary tagged mail
		}
	}
	name, ref := "Micro", ""
	switch {
	case a != nil:
		name, ref = a.Name, a.ID
	case platID != "":
		name, ref = platName, platID
	}
	started := time.Now()
	trigger := "email from " + m.From

	domain := mail.ConfiguredDomain()
	// Reply from the address that reaches this agent again, so hitting
	// reply continues the conversation. It used to answer from
	// agent@<domain> whoever had written to, which was a dead letter
	// until that address started resolving, and still loses which
	// agent you were talking to.
	// Answer from the address they wrote to. Whatever reached the agent is
	// what answers as it — anything else is a different sender arriving out
	// of nowhere, which is both confusing and a spam signal.
	//
	// This was reconstructed from the agent that answered rather than taken
	// from the message, so writing to agent@ got a reply from
	// agent+micro@: the catch-all resolves to the agent named micro, and
	// naming it in the reply address changed the address mid-conversation.
	from := m.To
	if from == "" {
		// Nothing to answer as. Older messages have no recipient recorded.
		from = mail.SharedAgentAddress()
		switch {
		case platID != "":
			from = agent.PlatformAddress(platID)
		case a != nil && a.Address() != "":
			from = a.Address()
		}
	}

	// Whether this message is for the agent at all.
	//
	// It always is when nobody else is on it — that is what writing to an agent
	// means. Once the agent has been copied into a conversation between other
	// people, answering every message would be a model call a turn and an
	// interruption a turn, which is how a thing gets muted. It speaks when it is
	// spoken to and when it has just arrived; otherwise it listens. See
	// thread.go and mail.Addressed.
	//
	// Nothing is lost by staying quiet: the message is already stored and
	// already in the record, so the next question it *is* asked has the whole
	// conversation behind it.
	if !wanted(m.Owner, m, from) {
		app.Log("mail", "agent %s is on this thread but was not addressed; staying quiet", name)
		return
	}

	// record writes the run down where the owner can find it, and hands
	// back the id so a failed delivery can be marked against it. Somebody
	// else's mail can start this run and spend this account's credits, so
	// the account has to be able to see that it happened.
	via := agent.Via{From: m.From}

	// The conversation the answer will be recorded on, filled in once the run
	// has happened. deliver closes over it so it can note the id the reply went
	// out under, which is what the next message in the thread looks for.
	var threadRef string

	record := func(prompt, answer string, err error) string {
		return agent.Record(agent.Recorded{
			Account: m.Owner, Agent: ref,
			Source: Client, Trigger: trigger,
			Prompt: prompt, Answer: answer, Err: err, Started: started,
			Via: agent.Via{Client: Client, From: m.From},
		})
	}

	// deliver sends an answer. flowID names the run it came from; "" is for the
	// replies that are not runs — a typo in the address, an account that cannot
	// pay — which still have to be written down and still have to be sent.
	deliver := func(flowID, prompt, body string) {
		if domain == "" || domain == "localhost" || from == "" {
			record(prompt, body, fmt.Errorf("this instance cannot send mail, so the answer went nowhere"))
			return
		}
		// An empty answer is not an answer. The agent returning "" is
		// not an error, so a blank body went all the way out as a
		// blank email — which reads as the agent having broken, and
		// tells the person nothing about what to do next.
		if strings.TrimSpace(body) == "" {
			app.Log("mail", "agent %s had nothing to say to %s; not sending", name, m.From)
			return
		}
		subject := m.Subject
		if !strings.HasPrefix(strings.ToLower(subject), "re:") {
			subject = "Re: " + subject
		}
		id := flowID
		if id == "" {
			id = record(prompt, body, nil)
		}
		// An agent answers in markdown, and mail was the one surface that
		// sent it raw — so an answer with a list or a bold word arrived
		// with its asterisks showing. Every other surface normalises and
		// renders; this is the same two steps, and RenderString is already
		// what the inbox uses for markdown arriving the other way.
		//
		// Render, not RenderTrusted: the body is model output, so raw HTML
		// in it is escaped rather than passed through.
		plain := app.NormalizeAnswerMarkdown(body)
		// Everybody on the thread, and the line that says what this is the
		// first time somebody else is in the room. See thread.go — the
		// introduction is empty for an ordinary one-to-one message, which is
		// every message from somebody writing to their own agent.
		to, cc := replyTo(m)
		plain = introduction(m.Owner, m, from) + plain
		sent, err := mail.SendExternalReplyAll(name, from, to, cc, subject,
			plain, app.RenderString(plain), m.MessageID, m.References)
		// The id the answer went out under, so the reply to *it* finds this
		// turn. Recorded even when delivery failed below, because a message
		// that reached the far side and then errored still gets answered.
		// Against the conversation, which is what the next message looks
		// in — the workflow record is how this answer was produced, not what
		// was said.
		agent.Sent(m.Owner, threadRef, sent)
		if err != nil {
			app.Log("mail", "agent %s could not reply to %s: %v", name, m.From, err)
			// Recorded as an error against the run, because a reply that
			// was written and never delivered is not a reply, and the
			// owner should not read the record as though it arrived.
			agent.Failed(id, err)
		}
	}

	// The message as prose. Exactly what the recorder wrote down, because they
	// are the same message — see asked.
	prompt := asked(m)
	if prompt == "" {
		return
	}

	// Written to a name that is not one of yours. Answered rather than
	// dropped: the person spelled out which agent they wanted, so they are
	// waiting for that agent and a typo should say so rather than look like
	// the agent having nothing to say.
	if unknownTag != "" {
		answer := fmt.Sprintf("There is no agent called %q here. The ones on "+
			"this instance are: %s — write to agent+<name>@%s, or to agent@%s "+
			"for Micro, which handles anything.",
			unknownTag, strings.Join(agent.PlatformNames(), ", "), domain, domain)
		// Your own agents are a different namespace and a different
		// address, so naming them here is the difference between a dead end
		// and a correction — the name they wanted may well exist, one
		// address over.
		var mine []string
		for _, own := range agent.Agents(m.Owner) {
			if own.Tag != "" {
				mine = append(mine, own.Tag)
			}
		}
		if len(mine) > 0 {
			answer += fmt.Sprintf("\n\nYour own agents are at %s+<name>@%s — "+
				"you have: %s.", mail.Handle(m.Owner, ""), domain, strings.Join(mine, ", "))
		}
		deliver("", prompt, answer)
		return
	}

	// Two ways to be allowed to run, and which one applies is not the caller's
	// choice: an account somebody signed up for spends credits, one opened for a
	// stranger who wrote in spends free turns. Asking an unclaimed account for
	// credits would answer a first-time sender with "top up at /account/topup"
	// — a bill, for an account they do not know exists. See trial.go.
	trial, allowed, why := trialRun(m.Owner)
	switch {
	case trial && !allowed:
		deliver("", prompt, why)
		return
	case !trial:
		canProceed, _, cost, err := quota.CheckQuota(m.Owner, quota.OpAgentQuery)
		if err != nil || !canProceed {
			deliver("", prompt, fmt.Sprintf("I could not run this one: it costs %d credits and the account is short. "+
				"Top up at %s/account/topup and send it again.", cost, app.PublicURL()))
			return
		}
	}

	// The same entry point every client uses: history, the run record and
	// anything worth remembering are its business. What is passed is what only
	// mail knows — which turn this answers, and the ids that will find it again.
	res, err := agent.Ask(agent.AskRequest{
		Account: m.Owner,
		Client:  Client,
		Thread:  chainKey(m),
		Text:    prompt,
		Agent:   ref,
		// Told whether it is alone with the sender or copied into somebody
		// else's conversation, because everything about how it should write
		// changes with that. See agent.GroupPrompt.
		System:     agent.GroupPrompt(m.Others),
		Trigger:    trigger,
		As:         from,
		Ref:        m.InReplyTo + " " + m.References,
		MessageRef: m.MessageID,
		From:       m.From,
		FromName:   m.FromName,
		Via:        via,
	})
	answer := res.Text
	threadRef = res.Thread
	if err != nil {
		app.Log("mail", "agent %s failed on mail from %s: %v", name, m.From, err)
		deliver(res.Flow, prompt, "I could not answer that one. Try again, or ask a different way.")
		return
	}
	// Charged now the run has happened. CheckQuota above only asks whether it
	// can be afforded — nothing here consumed it, so every agent run started by
	// mail was free, which is the one door somebody else can push.
	//
	// A trial run is spent against turns instead, and never against the ledger:
	// an unclaimed account has no balance to debit and debiting it would put it
	// in the red before anybody had agreed to anything.
	if trial {
		defer trialSpent(m.Owner)
	} else {
		quota.ConsumeQuota(m.Owner, quota.OpAgentQuery) //nolint:errcheck
	}
	if strings.TrimSpace(answer) == "" {
		// Distinct from the error above, because it is a different
		// fact: the run finished and produced nothing. Silence would
		// leave somebody watching an inbox for a reply that is never
		// coming.
		app.Log("mail", "agent %s produced an empty answer for mail from %s", name, m.From)
		deliver(res.Flow, prompt, "I did not manage an answer to that one. Try asking a different way.")
		return
	}
	deliver(res.Flow, prompt, answer)
}
