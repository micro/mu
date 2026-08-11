// Package whatsapp is WhatsApp as something an agent can send, rather than only
// a place a person can reach one.
//
// There was already a WhatsApp client — Meta's Cloud API, a bot loop that turns
// an arriving message into a question for the agent. That is the door. This is
// the other half: a capability, so "let them know on WhatsApp" is a thing an
// agent can do rather than a thing that only happens to it. Every medium worth
// having has both halves, and which one exists so far has mostly been an
// accident of where somebody was working.
//
// It goes through Twilio, which sends WhatsApp over the same endpoint as a text
// with whatsapp: on the addresses — so this is the rules and the bookkeeping,
// and internal/twilio is the wire.
//
// The rule that shapes everything here is not ours. WhatsApp only allows a
// free-form message inside twenty-four hours of the recipient's last message;
// outside that window a business may only send a template approved in advance.
// This instance has no approved templates, so the honest form of that rule is:
// you may reply to people, and you may not start conversations. Enforced here
// rather than left to fail at the provider, because a refusal that explains
// itself beats error 63016.
package whatsapp

import (
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/settings"
	"mu/internal/twilio"
	"mu/internal/userdb"
	"mu/service/sms"
)

const (
	ns       = "whatsapp"
	msgs     = "messages"
	routes   = "routes"
	instance = "instance"

	// window is how long after somebody writes to you that you may write back
	// freely. WhatsApp's rule, not this instance's.
	window = 24 * time.Hour

	maxBody = 1500
)

// Message is one WhatsApp message, in or out.
type Message struct {
	ID        string    `json:"id"`
	Direction string    `json:"direction"` // "out" or "in"
	Number    string    `json:"number"`    // the other end, E.164 without the whatsapp: prefix
	Text      string    `json:"text"`
	At        time.Time `json:"at"`
}

// From is the WhatsApp sender this instance uses, in plain E.164.
func From() string { return sms.Normalise(settings.Get("TWILIO_WHATSAPP_FROM")) }

// Configured reports whether this instance can send WhatsApp at all.
func Configured() bool {
	if user, pass := twilio.Credentials(); user == "" || pass == "" {
		return false
	}
	return From() != ""
}

// address is how Twilio wants a WhatsApp party written.
func address(number string) string { return "whatsapp:" + number }

// number strips the prefix off one.
func number(address string) string {
	return sms.Normalise(strings.TrimPrefix(strings.TrimSpace(address), "whatsapp:"))
}

// ── The window ──────────────────────────────────────────────────

// OpenUntil is when the window on a conversation closes, or the zero time if it
// is shut.
//
// It opens each time they write, which is why a reply is always allowed and a
// cold approach never is. Nothing here can widen it: it is Meta's rule, and a
// message sent outside it is refused by the provider whatever this thinks.
func OpenUntil(owner, num string) time.Time {
	var last time.Time
	for _, m := range History(owner, 200) {
		if m.Direction == "in" && m.Number == num && m.At.After(last) {
			last = m.At
		}
	}
	if last.IsZero() {
		return time.Time{}
	}
	return last.Add(window)
}

// Open reports whether this owner may write to this number right now.
func Open(owner, num string) bool {
	until := OpenUntil(owner, num)
	return !until.IsZero() && time.Now().Before(until)
}

// Fresh reports whether a message would start a new billable conversation.
//
// Twilio bills WhatsApp by the twenty-four hour conversation rather than the
// message, so charging per message would overcharge a long thread several times
// over for one thing we were billed for once. The first message inside a window
// is the one that costs.
func Fresh(owner, num string) bool {
	cutoff := time.Now().Add(-window)
	for _, m := range History(owner, 200) {
		if m.Direction == "out" && m.Number == num && m.At.After(cutoff) {
			return false
		}
	}
	return true
}

// ── Messages ────────────────────────────────────────────────────

// Record stores a message. Direction is "out" or "in".
func Record(owner, direction, num, text string) *Message {
	m := &Message{Direction: direction, Number: num, Text: text, At: time.Now()}
	if direction == "out" {
		route(owner, num)
	}
	rec, err := userdb.Create(ns, owner, msgs, map[string]interface{}{
		"direction": direction, "number": num, "text": text,
		"at": m.At.Format(time.RFC3339),
	}, false)
	if err != nil {
		app.Log("whatsapp", "storing %s message for %s: %v", direction, owner, err)
		return m
	}
	m.ID = rec.ID
	return m
}

// History returns this owner's messages, newest first.
func History(owner string, limit int) []Message {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	recs, err := userdb.List(ns, owner, msgs, "mine", nil, "", "", limit)
	if err != nil {
		return nil
	}
	out := make([]Message, 0, len(recs))
	for _, r := range recs {
		m := Message{ID: r.ID}
		m.Direction, _ = r.Data["direction"].(string)
		m.Number, _ = r.Data["number"].(string)
		m.Text, _ = r.Data["text"].(string)
		if s, ok := r.Data["at"].(string); ok {
			m.At, _ = time.Parse(time.RFC3339, s)
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// ── Who an arriving message belongs to ──────────────────────────

func route(owner, num string) {
	recs, err := userdb.List(ns, instance, routes, "mine",
		map[string]interface{}{"number": num}, "", "", 1)
	data := map[string]interface{}{"number": num, "owner": owner, "at": time.Now().Format(time.RFC3339)}
	if err == nil && len(recs) == 1 {
		userdb.Update(ns, instance, routes, recs[0].ID, data, false) //nolint:errcheck
		return
	}
	userdb.Create(ns, instance, routes, data, false) //nolint:errcheck
}

// OwnerOf finds which account an arriving message belongs to.
//
// Harder than it is for a text, and the reason is the window: a WhatsApp
// conversation always starts with the other person, so the usual answer —
// whoever wrote to them last from here — does not exist yet the first time.
// Somebody who has proved that number is theirs is the answer that does, and
// that proof already exists next door: a number verified for texts is the same
// number. Failing that, the last conversation. Failing that, nobody, and the
// message is dropped rather than handed to whoever happens to be first.
func OwnerOf(num string) string {
	if owner := sms.NumberOwner(num); owner != "" {
		return owner
	}
	recs, err := userdb.List(ns, instance, routes, "mine",
		map[string]interface{}{"number": num}, "", "", 1)
	if err == nil && len(recs) > 0 {
		if owner, _ := recs[0].Data["owner"].(string); owner != "" {
			return owner
		}
	}
	// Whoever runs the instance owns its number, so an unclaimed message is
	// theirs to see. Dropping it was tidier and lost messages.
	return sms.Fallback()
}

// DeleteAll removes everything whatsapp holds for an owner.
func DeleteAll(owner string) {
	if owner == "" || owner == instance {
		return
	}
	if n, err := userdb.DeleteOwner(ns, owner); err != nil {
		app.Log("whatsapp", "deleting %s's messages: %v", owner, err)
	} else if n > 0 {
		app.Log("whatsapp", "deleted %d whatsapp records for %s", n, owner)
	}
}
