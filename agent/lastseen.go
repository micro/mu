package agent

import (
	"mu/internal/thread"
	"strings"
	"time"
)

type lastSeen struct {
	subject string
	at      time.Time
}

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
// trimTo cuts a line to n runes, with an ellipsis where it cut.
func trimTo(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
