package digest

// The briefing arrives in the inbox.
//
// It was published as a blog post and nothing else, which means the one thing
// the agent does on its own every day was visible only to somebody who went
// looking for it. An inbox that only ever contains conversations you started is
// not an inbox — it is a chat history. This is the first thing that turns up in
// it without being asked for, which is the whole claim: the agent works whether
// or not you have the page open.
//
// It goes into the record rather than out over SMTP. Nothing is sent to anybody
// — a brief lands in the inbox on the instance the account already signed up
// for, the same way it lands on the blog. Mailing it is a different decision
// with a different cost, and it can be made later without undoing this.
//
// One conversation per account per day, so replying to it is replying to that
// day's brief and the next one does not land in the middle of the thread.

import (
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
	"mu/internal/thread"
)

// Client is where a brief appears to have come from, in the record.
//
// Its own name rather than the web or mail, because neither is true: nobody
// typed it and no MTA delivered it. A reader asking "where did this come from"
// gets an answer that means something — see app.ClientName.
const Client = "digest"

// deliver puts the day's briefing in every account's inbox.
//
// Off with DIGEST_INBOX=false, because an operator running this for other
// people should be able to decide that a daily arrival is not what their
// account holders signed up for. On by default: the brief is what the instance
// is for, and a feature nobody can find is the failure this fixes.
func deliver(title, body string) {
	if !toInbox() {
		return
	}
	if body == "" {
		return
	}

	// The day, so a second run on the same day continues the same conversation
	// rather than starting a second one. thread.Open is find-or-create on this
	// key, so a retry after a failure is not a duplicate.
	key := "digest-" + time.Now().Format("2006-01-02")

	sent := 0
	for _, acc := range auth.AllAccounts() {
		if acc == nil || acc.ID == "" || acc.Banned {
			continue
		}
		th := thread.Open(acc.ID, Client, key)
		if th == nil {
			continue
		}
		// Already delivered to this account today. Open found the conversation
		// rather than making it, and it has the brief in it.
		if len(thread.Messages(acc.ID, th.ID, 1)) > 0 {
			continue
		}
		if thread.Add(thread.Message{
			Thread:  th.ID,
			Account: acc.ID,
			Role:    thread.RoleAgent,
			Text:    title + "\n\n" + body,
		}) != "" {
			sent++
		}
	}
	if sent > 0 {
		app.Log("digest", "Briefing delivered to %d inbox%s", sent, plural(sent))
	}
}

func toInbox() bool {
	switch settings.Get("DIGEST_INBOX") {
	case "false", "0", "off", "no":
		return false
	}
	return true
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}
