package agent

// Your agents, on the screen you arrive at.
//
// Home had the inbox and the services and nothing between them, and the two
// are the wrong pair on their own: what arrived, and what the instance knows.
// The thing missing was what you have working on it.
//
// A runs block was here once and was removed for a good reason — "one run is a
// receipt for something you just watched happen". This is not that. A run is an
// event and ages out; an agent is a standing thing, and what belongs on Home is
// the roster with a sign of life against each one: who you have, what they last
// dealt with, and a way straight into any of them.
//
// Last activity comes from internal/thread rather than from a workflow record,
// which is the same distinction: agent.Flow is how an answer was produced and
// expires, a conversation is what was said and does not. "Last spoke about X,
// two hours ago" survives an eviction; "ran six tools in 4.2s" does not, and
// was never the thing anybody wanted to know from the front page.
//
// # The default is on it
//
// This listed Agents(owner), which is an account's *own* agents — so the one
// every account already has was missing, on the page whose whole job is to say
// what you have working. /agents had the same bug once and its fix is recorded
// there: leaving Micro off "meant a new account opened /agents and was told it
// had none, which is false". Both pages prepend it now, from the same place,
// because two lists of your agents that disagree about how many there are is
// worse than either.

import (
	"html"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/thread"
)

// lastSeen is the most recent conversation an agent is on.
//
// # The subject, and why it is the first message
//
// A thread is named by how it started: thread.Add derives a subject from the
// first message and thread.Name takes a mail Subject once, both deliberately
// first-one-wins. So a conversation opened with "Hello" on Monday and used for
// something real on Friday reads here as "Hello", beside a timestamp saying
// Friday — which looks wrong, and was changed to the newest message for a day
// on that reading.
//
// It was changed back. The two halves are not one fact and do not have to
// agree: the subject is what the conversation is *called* and Updated is when
// it last moved, which is exactly the pair every mail client puts on a row. The
// last message is worse at the job — an agent's answer clipped to sixty
// characters is a fragment, and the same conversation would be labelled
// differently every time it was looked at.
type lastSeen struct {
	subject string
	at      time.Time
}

// previewShown is how many agents Home carries. Enough to see what you have,
// short of turning the front page into the roster.
const previewShown = 5

// previewSubject bounds the line of text under a name.
const previewSubject = 60

// entry is one row: an agent, however it came to exist.
type entryOf struct {
	ID   string
	Name string
	Path string
}

// Preview is your agents and what they last dealt with, for Home. Empty when
// there are none — a heading over nothing says less than no heading.
func Preview(accountID string) string {
	if accountID == "" {
		return ""
	}

	// The default first, and pinned there rather than sorted with the rest.
	// It is the one the chat talks to and the one that answers agent@, so it is
	// what somebody is looking for; /agents puts it in the same place for the
	// same reason.
	var rows []entryOf
	if a := Platform(DefaultPlatformAgent); a != nil {
		rows = append(rows, entryOf{
			ID:   DefaultPlatformAgent,
			Name: a.Name,
			Path: "/agent/" + DefaultPlatformAgent,
		})
	}

	own := Agents(accountID)
	if len(rows) == 0 && len(own) == 0 {
		return ""
	}

	latest := latestByAgent(accountID)

	// Busiest first, which on this page means most recently used — an agent you
	// have not touched in a month is not what you came to look at. Ones that
	// have done nothing keep their roster order underneath. The default is
	// already at the top and does not join this.
	sorted := make([]*Agent, len(own))
	copy(sorted, own)
	sortByRecent(sorted, latest)
	for _, a := range sorted {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = a.ID
		}
		// Path takes the owner, and it was given "". SlugFor then looks the id up
		// in Agents(""), which is nobody's roster, finds nothing, decides it must
		// be one of the instance's own, finds nothing again, and falls back to the
		// default slug — so every agent on this page linked to /agent/micro. A
		// lookup miss that resolves to a different agent rather than to an error
		// is the shape of it: the link worked, it just went somewhere else.
		rows = append(rows, entryOf{ID: a.ID, Name: name, Path: Path(accountID, a.ID)})
	}

	if len(rows) > previewShown {
		rows = rows[:previewShown]
	}

	var b strings.Builder
	b.WriteString(`<div class="agent-peek">`)
	for _, a := range rows {
		b.WriteString(`<a class="agent-peek-row" href="` + html.EscapeString(a.Path) + `">`)
		b.WriteString(`<span class="agent-peek-name">` + html.EscapeString(a.Name) + `</span>`)

		if s, ok := latest[a.ID]; ok {
			what := s.subject
			if what == "" {
				what = "a conversation"
			}
			b.WriteString(`<span class="agent-peek-last">` +
				html.EscapeString(trimTo(what, previewSubject)) + `</span>`)
			b.WriteString(`<span class="agent-peek-when">` +
				html.EscapeString(app.TimeAgo(s.at)) + `</span>`)
		} else {
			// Nothing yet, said as a state rather than left blank — a row with
			// an empty middle reads as a render that failed.
			b.WriteString(`<span class="agent-peek-last agent-peek-idle">Nothing yet</span>`)
		}
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// latestByAgent is the last conversation each agent actually spoke on.
//
// One pass over the record rather than a lookup per agent: thread.List is
// already sorted newest first, so the first thread naming an agent is that
// agent's latest.
//
// Shared with /agents, which asks the same question about the same agents —
// two lists of what your agents have been doing that disagreed about it would
// be worse than either.
func latestByAgent(accountID string) map[string]lastSeen {
	latest := map[string]lastSeen{}
	for _, t := range thread.List(accountID, previewHistory) {
		// Something an agent actually did.
		//
		// This read every conversation, and most conversations have no agent
		// recorded on them — so the line below filed all of them under the
		// default and Micro's row reported, as the last thing it dealt with, a
		// DMARC aggregate report from Google that nothing had read. Mail the
		// agent deliberately stayed quiet on did the same: it is on a thread
		// between other people and says so, and the front page turned that
		// silence into activity.
		//
		// A party of RoleAgent is the exact fact wanted, and it is already
		// there: parties accrete from who spoke, so an agent is on a
		// conversation when it has answered on it and not before.
		if !answered(accountID, t) {
			continue
		}
		// A conversation with nobody named is the default's: the chat records
		// whichever agent answered, and for the one that answers when none was
		// asked for that is empty.
		who := t.Agent
		if who == "" {
			who = DefaultPlatformAgent
		}
		if _, have := latest[who]; have {
			continue
		}
		latest[who] = lastSeen{subject: strings.TrimSpace(t.Subject), at: t.Updated}
	}
	return latest
}

// LastSeen is what an agent last dealt with and when, for a list that is about
// picking one to talk to. Empty when it has not answered anything yet.
//
// Exported for /agents. The roster there was a token inventory — name, purpose,
// three links — and nothing on it said whether any of them were alive. "Last:
// Tuesday · 2 hours ago" is what turns a directory entry into somebody you are
// about to talk to.
func LastSeen(accountID, agentID string) (about string, at time.Time) {
	if accountID == "" {
		return "", time.Time{}
	}
	if agentID == "" {
		agentID = DefaultPlatformAgent
	}
	s, ok := latestByAgent(accountID)[agentID]
	if !ok {
		return "", time.Time{}
	}
	return s.subject, s.at
}

// answered says whether an agent has spoken on a conversation.
//
// Not whether one is named on it. t.Agent is who a conversation is *with*,
// which is set from whoever would answer — an email addressed to an agent is
// with that agent from the moment it lands, answered or not. What Home is
// reporting is activity, and "arrived" is not activity.
func answered(accountID string, t thread.Thread) bool {
	for _, p := range thread.Parties(accountID, t.ID) {
		if p.Kind == thread.RoleAgent {
			return true
		}
	}
	return false
}

// previewHistory is how far back to look for the last thing each agent did. Far
// enough that an agent used last week still shows one, short of walking the
// whole record to render a front page.
const previewHistory = 200

// sortByRecent puts the agents that have done something most recently first,
// and leaves the rest in the order the roster gave them.
func sortByRecent(agents []*Agent, latest map[string]lastSeen) {
	// A stable insertion sort: the list is at most a few dozen, and stability
	// is what keeps the untouched ones in roster order rather than shuffling
	// them every load.
	for i := 1; i < len(agents); i++ {
		for j := i; j > 0; j-- {
			a, ok := latest[agents[j].ID]
			b, okPrev := latest[agents[j-1].ID]
			if !ok {
				break // nothing to rank it by, so it stays where it is
			}
			if okPrev && !a.at.After(b.at) {
				break
			}
			agents[j], agents[j-1] = agents[j-1], agents[j]
		}
	}
}

// trimTo cuts a line to n runes, with an ellipsis where it cut.
func trimTo(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
