package sms

// What became of a message after we handed it over.
//
// Sending is not delivering. twilio.Send returns the moment the provider has
// accepted the message, which on a long code or an unregistered sender can be
// most of a minute before a carrier puts it on somebody's phone — and for that
// whole minute the message reads "sent" here and the person waiting reads it as
// broken. "It was slow" then has no answer, because the record stops at the
// handover: the agent's own half is timed in the run record and everything
// after it was unmeasured.
//
// Twilio will say, if asked. StatusCallback on the send is a URL it posts to as
// the message moves — queued, sent, delivered, undelivered, failed — and this
// records the last thing it said with the time it said it. Two timestamps on
// one message is the whole diagnosis: At is when we let go, StatusAt is when it
// landed, and whichever of those two gaps is the big one is the one worth
// working on. Guessing between them is what this replaces.
//
// The callback names the message by the provider's id and nothing else — no
// account, no session — so the id has to be findable across the whole instance.
// That is what sends is: the same instance-scoped index route already uses for
// the same reason, because a message belongs to its owner and one account
// cannot read another's.

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/settings"
	"mu/internal/userdb"
)

// statusOf is the states a receipt can report, as the provider spells them.
//
// A closed set rather than whatever arrives, because this is written into
// people's message history by a public endpoint and the field is rendered.
var statusOf = map[string]bool{
	"accepted": true, "scheduled": true, "queued": true, "sending": true,
	"sent": true, "receiving": true, "received": true, "delivered": true,
	"undelivered": true, "failed": true, "read": true, "canceled": true,
}

// settled reports whether a status is the last word on a message. Anything
// before it is the provider still working, and a later receipt will replace it.
func settled(status string) bool {
	switch status {
	case "delivered", "read", "undelivered", "failed", "canceled":
		return true
	}
	return false
}

// callbackURL is where the provider should post what became of a message, or ""
// when this instance has no address it can be reached at.
//
// Derived rather than configured. The operator has already said where this
// instance lives — twice, if they had to set TWILIO_WEBHOOK_URL to get the
// signature check to pass — and a third setting saying it again is a third
// place for the three to disagree. TWILIO_WEBHOOK_URL wins where it is set,
// because it is the address that is known to reach us.
func callbackURL() string {
	if u := strings.TrimSpace(settings.Get("TWILIO_WEBHOOK_URL")); u != "" {
		if i := strings.Index(u, "://"); i >= 0 {
			if j := strings.Index(u[i+3:], "/"); j >= 0 {
				return u[:i+3+j] + "/sms/status"
			}
			return strings.TrimSuffix(u, "/") + "/sms/status"
		}
	}
	d := strings.TrimSpace(settings.Get("MU_DOMAIN"))
	d = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://"), "/")
	if d == "" || d == "localhost" || strings.HasPrefix(d, "localhost:") {
		// Nothing Twilio can reach, so do not ask it to try. An unreachable
		// callback is not an error there, it is a retry loop against somebody.
		return ""
	}
	return "https://" + d + "/sms/status"
}

// noteSend files where a sent message is, under the provider's id for it.
//
// Instance-scoped, like route, and for the same reason: a receipt arrives with
// no account on it, and the owner's own records are the one place it cannot be
// looked up.
func noteSend(owner, id, sid string) {
	if owner == "" || id == "" || sid == "" {
		return
	}
	userdb.Create(ns, instance, sends, map[string]interface{}{
		"sid": sid, "owner": owner, "message": id,
		"at": time.Now().Format(time.RFC3339),
	}, false) //nolint:errcheck
}

// sentAs finds the message a provider id names.
func sentAs(sid string) (owner, id string) {
	if strings.TrimSpace(sid) == "" {
		return "", ""
	}
	recs, err := userdb.List(ns, instance, sends, "mine",
		map[string]interface{}{"sid": sid}, "", "", 1)
	if err != nil || len(recs) != 1 {
		return "", ""
	}
	owner, _ = recs[0].Data["owner"].(string)
	id, _ = recs[0].Data["message"].(string)
	return owner, id
}

// SetStatus records what the provider says became of a message, and reports
// whether there was a message to record it against.
//
// A receipt for something this instance did not send is not an error — Twilio
// retries, and a message from before this existed has no index entry — so it is
// dropped quietly rather than logged as a failure.
func SetStatus(sid, status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	if !statusOf[status] {
		return false
	}
	owner, id := sentAs(sid)
	if owner == "" || id == "" {
		return false
	}
	cur, err := userdb.Get(ns, owner, msgs, id)
	if err != nil || cur == nil {
		return false
	}
	// Receipts do not arrive in order — sent and delivered are two posts and
	// either can be the one that is retried — so a settled message is not
	// walked backwards to "sending" by a late one.
	if was, _ := cur.Data["status"].(string); settled(was) && !settled(status) {
		return true
	}
	// Read, merge, write. Update replaces the record's data outright rather
	// than merging into it, so a map of the two fields being set would be the
	// message with its text, number and channel deleted.
	next := make(map[string]interface{}, len(cur.Data)+2)
	for k, v := range cur.Data {
		next[k] = v
	}
	next["status"] = status
	next["status_at"] = time.Now().Format(time.RFC3339)
	if _, err := userdb.Update(ns, owner, msgs, id, next, cur.Public); err != nil {
		app.Log("sms", "recording %s for message %s: %v", status, sid, err)
		return false
	}
	return true
}

// slow is how long a message may take to land before the delay is the story.
//
// Half a minute. Under it nobody notices and saying so is noise on every line;
// over it somebody was sitting looking at a phone wondering whether this works,
// and that is worth a line on the page.
const slow = 30 * time.Second

// stuck is how long a message may sit with no receipt at all before that is
// itself the report. Two minutes of nothing is a message the carrier is holding
// or a callback that never reaches us, and both are worth seeing.
const stuck = 2 * time.Minute

// deliveryNote is what to say about a sent message's fate, or "" when there is
// nothing worth saying.
//
// Quiet by default, on purpose. A green tick beside every message that worked
// is a page about itself; the whole reason this exists is the messages that did
// not work, or took long enough that somebody noticed.
func deliveryNote(m Message) string {
	if m.Direction != "out" {
		return ""
	}
	switch m.Status {
	case "failed", "undelivered", "canceled":
		return "not delivered"
	case "delivered", "read":
		if took := m.Took(); took >= slow {
			return "delivered after " + spell(took)
		}
		return ""
	case "":
		// Nothing was ever reported. Either the provider has not said yet, or
		// this instance is not being told — see callbackURL.
		if time.Since(m.At) > stuck && callbackURL() != "" {
			return "no delivery receipt"
		}
		return ""
	}
	// Accepted, queued, sending, sent: still on its way, which is only news
	// once it has been on its way for a while.
	if time.Since(m.At) > stuck {
		return m.Status + " for " + spell(time.Since(m.At))
	}
	return ""
}

// spell writes a delay the way somebody would say it. Whole units only: the
// difference between 42 and 42.3 seconds is not a thing anybody is deciding
// anything on.
func spell(d time.Duration) string {
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Round(time.Second)/time.Second)) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Round(time.Minute)/time.Minute)) + "m"
	default:
		return strconv.Itoa(int(d.Round(time.Hour)/time.Hour)) + "h"
	}
}

// StatusHandler receives a delivery receipt from Twilio.
//
// The same door as the inbound webhook and the same credential: this is a
// public URL that writes into people's message history, so the provider's
// signature is what says the receipt is real. Where there is no auth token to
// check one against, implausible does what it can — the receipt names this
// instance's own account and sender, and a post that names neither is not worth
// the benefit of the doubt.
//
// It answers 200 whatever happens. A receipt is a fact about a message that has
// already gone; there is nothing for the provider to do about our opinion of
// it, and an error here only buys a retry of the same post.
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !verifyInbound() {
		if why := implausible(r); why != "" {
			app.Log("sms", "unverified delivery receipt refused: %s", why)
			http.Error(w, "forbidden: "+why, http.StatusForbidden)
			return
		}
	} else if !validSignature(r, signedURLs(r), r.PostForm) {
		app.Log("sms", "delivery receipt rejected: signature did not match")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	sid := strings.TrimSpace(r.PostForm.Get("MessageSid"))
	status := strings.TrimSpace(r.PostForm.Get("MessageStatus"))
	if code := strings.TrimSpace(r.PostForm.Get("ErrorCode")); code != "" && code != "0" {
		// The reason is the useful half of a failure and it is not in the
		// status, which says only that it did not arrive.
		app.Log("sms", "message %s %s, provider error %s", sid, status, code)
	}
	SetStatus(sid, status)
	twiml(w, "")
}
