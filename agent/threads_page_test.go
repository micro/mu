package agent

// The record is only kept if somebody can see it.
//
// Every client has been writing to internal/thread and nothing read it, which
// is the same state the clients were in when they each kept history in a map:
// indistinguishable from not keeping it at all until something goes looking.
// These pin what the page has to be — the record rather than the workflow
// store, every client rather than the web, and model output escaped.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"mu/internal/thread"
)

// The page reads the system of record, not the run log.
//
// Runs and Threads answer different questions and were one struct once. A
// conversation list assembled from workflow records is that mistake made again,
// and it is invisible until an old run is evicted and takes the conversation
// with it.
func TestTheThreadsPageReadsTheRecord(t *testing.T) {
	b, err := os.ReadFile("threads_page.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	for _, want := range []string{"thread.List(", "thread.Messages(", "thread.Get("} {
		if !strings.Contains(src, want) {
			t.Errorf("the threads page does not call %s", want)
		}
	}
	// getFlow is allowed — a message names the run that produced it and the page
	// shows what that run called. Listing conversations from flows is not.
	for _, gone := range []string{"ListSessions(", "ListFlows("} {
		if strings.Contains(src, gone) {
			t.Errorf("the threads page builds its list from %s — that is the workflow "+
				"store, which expires, so conversations would disappear when debugging "+
				"records are evicted", gone)
		}
	}
}

// And it is reachable from the agent surface.
func TestTheAgentSurfaceOffersThreads(t *testing.T) {
	tabs := agentTabs("chat", "")
	if !strings.Contains(tabs, `href="/agent/threads"`) {
		t.Error("no Threads tab — the record is written on every turn and there is no " +
			"way to look at it")
	}
	// Threads is not one agent's. A mail chain or a Discord DM is a conversation
	// with this instance, and scoping the tab to whichever agent a page happened
	// to be about would hide the rest of somebody's history.
	if strings.Contains(agentTabs("chat", "agent-123"), `href="/agent/threads?`) {
		t.Error("the Threads tab carries an agent — the record is not per agent")
	}
}

// A conversation reads as itself: who said what, on which client.
func TestAConversationRendersWhatWasSaid(t *testing.T) {
	owner := fmt.Sprintf("threads-page-%d", time.Now().UnixNano())

	th := thread.Open(owner, "mail", "<root@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: owner,
		Text: "what are your rates?", From: "someone@example.com"})
	thread.Add(thread.Message{Thread: th.ID, Account: owner, Role: thread.RoleAgent,
		Text: "**£500** a day."})

	msgs := thread.Messages(owner, th.ID, 0)
	if len(msgs) != 2 {
		t.Fatalf("recorded %d messages, want 2", len(msgs))
	}

	person := messageBlock(msgs[0])
	if !strings.Contains(person, "what are your rates?") {
		t.Error("what a person wrote is not on the page")
	}
	// Somebody else wrote in to an address this account owns. Rendering that as
	// "You" attributes a stranger's message to the account holder.
	if !strings.Contains(person, "someone@example.com") {
		t.Error("a message from somebody else is not attributed to them")
	}

	agentSaid := messageBlock(msgs[1])
	if !strings.Contains(agentSaid, "<strong>£500</strong>") {
		t.Errorf("the agent's answer is not rendered as markdown, so a reply reads as "+
			"asterisks — this was the observed bug in mail. Got: %s", agentSaid)
	}

	row := threadRow(owner, *thread.Get(owner, th.ID))
	if !strings.Contains(row, "/agent/threads?id="+th.ID) {
		t.Error("the row does not open the conversation")
	}
	if !strings.Contains(row, "Email") {
		t.Error("the row does not say which client it happened on, which is the whole " +
			"point of one record across five of them")
	}
}

// Model output is escaped, because it is untrusted.
//
// An agent's answer contains whatever a tool read off the open web. The chat
// renders it through the untrusted renderer for that reason, and a second page
// showing the same text has the same problem.
func TestAnAnswerCannotSmuggleMarkupOntoThePage(t *testing.T) {
	owner := fmt.Sprintf("threads-escape-%d", time.Now().UnixNano())

	th := thread.Open(owner, WebClient, "session-1")
	thread.Add(thread.Message{Thread: th.ID, Account: owner,
		Text: `<script>alert('typed')</script>`})
	thread.Add(thread.Message{Thread: th.ID, Account: owner, Role: thread.RoleAgent,
		Text: `<script>alert('answered')</script>`})

	for _, m := range thread.Messages(owner, th.ID, 0) {
		if strings.Contains(messageBlock(m), "<script>") {
			t.Errorf("a %s message put raw script on the page", m.Role)
		}
	}

	// And in the list, where the last message is shown as a preview.
	if strings.Contains(threadRow(owner, *thread.Get(owner, th.ID)), "<script>") {
		t.Error("the preview put raw script on the page")
	}
}

// Every client that writes to the record has a name in front of somebody.
func TestEveryClientIsNamed(t *testing.T) {
	for _, c := range []string{WebClient, "mail", "discord", "telegram", "whatsapp"} {
		if name := clientName(c); name == c {
			t.Errorf("%q is shown to people as %q — the client ids are directory names, "+
				"not labels", c, name)
		}
	}
	// An unknown one is shown as it names itself rather than dropped, so a client
	// written tomorrow appears on this page the day it is written.
	if clientName("newthing") != "newthing" {
		t.Error("an unrecognised client vanishes from the page")
	}
}
