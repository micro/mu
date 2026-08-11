// Package sms is a phone number: text somebody, and read what they text back.
//
// Mail is here because a sending domain is not something a caller can casually
// acquire — an SMTP server with DKIM and a reputation is weeks of work and a
// standing relationship with a provider. A phone number is the same argument
// and harder: you cannot get one without a company, a billing relationship and,
// in most countries, a registered sender identity. Email and the phone are the
// two addresses everybody has, and only one of them was here.
//
// What makes this different from every other service in the catalogue is that a
// mistake costs money and reputation in the same instant. A search that runs
// twice wastes a fraction of a penny. A text sent to the wrong number cannot be
// recalled, is charged either way, and a few thousand of them gets the sending
// number blocked by carriers — for everybody on the instance, not just whoever
// sent them. So the rules below are not defensive programming, they are the
// service:
//
//   - Only to countries the operator allows, because a text to a premium
//     destination costs fifty times what one to a mobile does, and those ranges
//     are where revenue-share fraud lives.
//   - A daily cap per account, tighter for an account nobody has vouched for
//     yet. Zero turns sending off instance-wide.
//   - The same message to the same number twice in a row is refused. A price
//     stops somebody who has to pay; it does nothing about a loop.
//   - STOP is honoured forever, per number, without asking anybody.
//   - Signed in only. An anonymous caller paying per message over x402 is still
//     a spammer, and the number's reputation is not something a payment can
//     make whole.
//
// There was one more, and it is gone: you could only text a number already in
// your contacts, verified as yours, or one that texted you first. It sounded
// careful and bought nothing — contacts_add takes a number and a name, so two
// calls defeated it, and meanwhile it made "text this person" a thing an agent
// could not do. It was also stricter than Twilio itself, which sits underneath
// and lets a funded account text anyone. A rule an attacker steps over and a
// user trips on is not a control.
//
// What replaces it is what actually holds: money, rate, consent and a ban
// button. SMS_KNOWN_ONLY brings it back for an operator who wants it.
//
// The price does the rest of the work; see quota.json.
package sms

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"strconv"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
	"mu/internal/userdb"
	"mu/service/contacts"
)

const (
	ns       = "sms"
	msgs     = "messages" // what was sent and received
	numbers  = "numbers"  // numbers this account has verified as its own
	optouts  = "optouts"  // numbers that said STOP — instance-wide
	routes   = "routes"   // number → the account an inbound reply belongs to
	instance = "instance" // what the instance owns rather than any account

	maxBody     = 480 // as long as one call may be
	maxSegments = 3   // and the most segments we will pay for in one
)

// Message is one text, in or out.
type Message struct {
	ID        string    `json:"id"`
	Direction string    `json:"direction"` // "out" or "in"
	Number    string    `json:"number"`    // the other end, always E.164
	Text      string    `json:"text"`
	Segments  int       `json:"segments"`
	At        time.Time `json:"at"`
}

// ── Numbers ─────────────────────────────────────────────────────

// Senders are the numbers this instance can send from, in the order configured.
//
// More than one, because one number does not serve two countries. A US long
// code texting a UK handset is filtered or dropped by UK carriers, and a UK
// number texting a US handset is blocked outright unless it is registered
// American traffic — so "send from our number" is only a sentence in a country
// that has a number. TWILIO_FROM takes a list, and the one matching the
// destination is used.
func Senders() []string {
	var out []string
	for _, part := range strings.Split(settings.Get("TWILIO_FROM"), ",") {
		if n := e164(part); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// From is the first configured number — what the page shows and what an agent
// is told to expect a reply on. FromFor is what actually sends.
func From() string {
	if s := Senders(); len(s) > 0 {
		return s[0]
	}
	return ""
}

// FromFor picks the number to text a destination from: the one in the same
// country, or nothing.
//
// Nothing, rather than falling back to whichever number is first. A message
// sent from the wrong country is charged at the international rate, arrives
// looking like a foreign stranger if it arrives at all, and cannot be replied
// to — a clear refusal is worth more than a delivery receipt for a message
// nobody read.
func FromFor(to string) string {
	digits := strings.TrimPrefix(e164(to), "+")
	best, bestLen := "", 0
	for _, cc := range allowedCountries() {
		if !strings.HasPrefix(digits, cc) {
			continue
		}
		for _, s := range Senders() {
			if strings.HasPrefix(strings.TrimPrefix(s, "+"), cc) && len(cc) > bestLen {
				best, bestLen = s, len(cc)
			}
		}
	}
	return best
}

// Ours reports whether a number is one this instance sends from.
func Ours(number string) bool {
	n := e164(number)
	for _, s := range Senders() {
		if s == n {
			return true
		}
	}
	return false
}

// messagingService is Twilio's own sender pool, if the operator uses one.
//
// It is the better answer for more than one country: Twilio holds the numbers,
// and with Geomatch on it picks the one whose country matches the handset —
// the same rule as FromFor, applied by the party that knows which of your
// numbers are registered for what. Set it and the numbers below are only used
// to say what a reply will come from.
func messagingService() string {
	return strings.TrimSpace(settings.Get("TWILIO_MESSAGING_SERVICE_SID"))
}

// Configured reports whether this instance can send at all.
func Configured() bool {
	if settings.Get("TWILIO_ACCOUNT_SID") == "" || settings.Get("TWILIO_AUTH_TOKEN") == "" {
		return false
	}
	return messagingService() != "" || len(Senders()) > 0
}

// e164 normalises a number to +<digits>.
//
// Everything downstream — the allowlist, the opt-out list, matching an inbound
// message to a contact — compares numbers as strings, so they have to be the
// same string. "+1 (555) 010-9999", "+15550109999" and "+1-555-010-9999" are
// one number and would otherwise be three.
func e164(s string) string {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		}
	}
	n := b.String()
	if n == "" || n == "+" {
		return ""
	}
	if !strings.HasPrefix(n, "+") {
		// A number with no country code is ambiguous, and guessing one is how
		// you text a stranger in another country. Only the instance's own
		// default rescues it.
		if cc := strings.TrimSpace(settings.Get("SMS_DEFAULT_COUNTRY")); cc != "" {
			n = "+" + strings.TrimPrefix(cc, "+") + strings.TrimPrefix(n, "0")
		} else {
			return ""
		}
	}
	if len(n) < 8 || len(n) > 16 {
		return ""
	}
	return n
}

// allowedCountries is the set of country codes this instance will text.
//
// A text to a UK or US mobile costs under a penny to a few pence. A text to
// some destinations costs thirty times that, and those destinations are where
// revenue-share fraud lives: the attacker owns the range and is paid for the
// traffic they trick you into sending. An allowlist is the only control that
// bounds the loss, because no per-message price is high enough for all of them.
func allowedCountries() []string {
	v := strings.TrimSpace(settings.Get("SMS_COUNTRIES"))
	if v == "" {
		// Cheap, well-policed destinations. An operator wanting more says so.
		v = "1,44,353,33,49,34,39,31"
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimPrefix(strings.TrimSpace(p), "+"); p != "" {
			out = append(out, p)
		}
	}
	// Longest first, so "1" cannot shadow a longer code it prefixes.
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func countryAllowed(number string) bool {
	digits := strings.TrimPrefix(number, "+")
	for _, cc := range allowedCountries() {
		if strings.HasPrefix(digits, cc) {
			return true
		}
	}
	return false
}

// ── Opt-out ─────────────────────────────────────────────────────

// stopWords are what somebody texts back to make it stop. Carriers require
// these to work, and a person who has said stop has said it to the number, not
// to whichever account happened to be behind it.
var stopWords = map[string]bool{
	"stop": true, "stopall": true, "unsubscribe": true,
	"cancel": true, "end": true, "quit": true,
}

// OptOut records that a number wants no more messages, from anybody here.
func OptOut(number string) {
	number = e164(number)
	if number == "" || OptedOut(number) {
		return
	}
	if _, err := userdb.Create(ns, instance, optouts,
		map[string]interface{}{"number": number, "at": time.Now().Format(time.RFC3339)},
		false); err != nil {
		app.Log("sms", "recording opt-out for %s: %v", number, err)
	}
}

// OptIn undoes an opt-out, which only the number itself can ask for.
func OptIn(number string) {
	number = e164(number)
	recs, err := userdb.List(ns, instance, optouts, "mine",
		map[string]interface{}{"number": number}, "", "", 5)
	if err != nil {
		return
	}
	for _, r := range recs {
		userdb.Delete(ns, instance, optouts, r.ID) //nolint:errcheck
	}
}

// OptedOut reports whether this number has said stop.
func OptedOut(number string) bool {
	recs, err := userdb.List(ns, instance, optouts, "mine",
		map[string]interface{}{"number": e164(number)}, "", "", 1)
	return err == nil && len(recs) > 0
}

// ── Who you may text ────────────────────────────────────────────

// Known reports whether this owner already has a relationship with a number.
//
// Three ways to have one, and none of them can be manufactured by the caller in
// the same breath as the send: somebody in the address book, a number the owner
// proved is theirs, or a number that texted them first. Anything else is a
// stranger, and texting strangers is the whole of the abuse.
func Known(owner, number string) bool {
	number = e164(number)
	if number == "" {
		return false
	}
	if Verified(owner, number) {
		return true
	}
	for _, c := range contacts.List(owner) {
		if e164(c.Phone) == number {
			return true
		}
	}
	recs, err := userdb.List(ns, owner, msgs, "mine",
		map[string]interface{}{"number": number, "direction": "in"}, "", "", 1)
	return err == nil && len(recs) > 0
}

// Verify marks a number as the owner's own.
func Verify(owner, number string) error {
	number = e164(number)
	if number == "" {
		return fmt.Errorf("that does not look like a phone number in international format, e.g. +447700900123")
	}
	if Verified(owner, number) {
		return nil
	}
	_, err := userdb.Create(ns, owner, numbers,
		map[string]interface{}{"number": number, "at": time.Now().Format(time.RFC3339)}, false)
	return err
}

// Verified reports whether this number belongs to this owner.
func Verified(owner, number string) bool {
	recs, err := userdb.List(ns, owner, numbers, "mine",
		map[string]interface{}{"number": e164(number)}, "", "", 1)
	return err == nil && len(recs) > 0
}

// Forget drops a number this owner had verified.
//
// Verifying is reversible, because a number is not yours forever: phones change
// hands, and a person who gave one up should be able to say so without an
// argument.
func Forget(owner, number string) {
	number = e164(number)
	recs, err := userdb.List(ns, owner, numbers, "mine",
		map[string]interface{}{"number": number}, "", "", 10)
	if err != nil {
		return
	}
	for _, r := range recs {
		userdb.Delete(ns, owner, numbers, r.ID) //nolint:errcheck
	}
}

// Numbers lists what this owner has verified as theirs.
func Numbers(owner string) []string {
	recs, err := userdb.List(ns, owner, numbers, "mine", nil, "", "", 50)
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range recs {
		if n, _ := r.Data["number"].(string); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// ── The daily cap ───────────────────────────────────────────────

// DailyLimit is how many messages one account may send in a day.
//
// The price is the first control and this is the second, because they fail
// differently: a price stops somebody who has to pay, a cap stops a loop that
// found a way not to. A runaway agent with a funded balance is the case that
// costs real money, and it is not a hypothetical.
//
// Set it to zero and nobody sends anything. That is the kill switch, and it is
// the same setting rather than a second one, because an operator reaching for
// it is in a hurry.
func DailyLimit() int { return limitSetting("SMS_DAILY_LIMIT", 20) }

// limitSetting reads a cap, and unlike app.EnvInt it believes a zero.
//
// EnvInt treats 0 as "not set" and hands back the default, which is right for
// a size and wrong for a limit: an operator typing zero into /admin/env to stop
// the texts would have been told twenty.
func limitSetting(key string, def int) int {
	v := strings.TrimSpace(settings.Get(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// LimitFor is the cap for one account.
//
// An account made in the last day gets a much smaller one. Signing up is free
// and takes a minute, so the cap on a fresh account is the only thing between
// a script and twenty texts an hour — and somebody genuinely texting a friend
// on their first day is not sending fifty.
func LimitFor(owner string) int {
	limit := DailyLimit()
	if limit == 0 {
		return 0
	}
	if auth.IsNewAccount(owner) {
		if n := limitSetting("SMS_NEW_ACCOUNT_LIMIT", 3); n < limit {
			return n
		}
	}
	return limit
}

// KnownOnly reports whether sending is restricted to numbers the caller already
// has a relationship with. Off by default — see the package comment.
func KnownOnly() bool {
	v := strings.ToLower(strings.TrimSpace(settings.Get("SMS_KNOWN_ONLY")))
	return v == "1" || v == "true" || v == "yes"
}

// Repeated reports whether this exact message went to this exact number a
// moment ago.
//
// The loop is the failure that costs money without anybody deciding to spend
// it: an agent that retries on a timeout, or one told to "keep them posted",
// sends the same sentence forty times and every one of them is charged and
// delivered. A person who genuinely wants to say the same thing twice can wait
// a few minutes or change a word.
func Repeated(owner, number, text string) bool {
	cutoff := time.Now().Add(-repeatWindow)
	for _, m := range History(owner, 20) {
		if m.At.Before(cutoff) {
			break
		}
		if m.Direction == "out" && m.Number == number && m.Text == text {
			return true
		}
	}
	return false
}

const repeatWindow = 5 * time.Minute

// SentToday counts what this owner has sent since midnight.
func SentToday(owner string) int {
	since := time.Now().Add(-24 * time.Hour)
	recs, err := userdb.List(ns, owner, msgs, "mine",
		map[string]interface{}{"direction": "out"}, "", "", 500)
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range recs {
		at, _ := r.Data["at"].(string)
		if t, err := time.Parse(time.RFC3339, at); err == nil && t.After(since) {
			n++
		}
	}
	return n
}

// ── Messages ────────────────────────────────────────────────────

// Segments is how many messages a body will actually be charged as.
//
// A text is 160 characters of GSM-7, or 70 of UCS-2 the moment one character
// falls outside that alphabet — an emoji, a curly quote pasted from a document.
// The provider bills per segment, so a 200-character message with one emoji in
// it is three messages and three times the price. Counting it here is what lets
// the caller be charged what it costs rather than what it looked like.
func Segments(text string) int {
	unicode := false
	for _, r := range text {
		if r > 127 {
			unicode = true
			break
		}
	}
	n := len([]rune(text))
	single, multi := 160, 153
	if unicode {
		single, multi = 70, 67
	}
	if n <= single {
		return 1
	}
	return (n + multi - 1) / multi
}

// Record stores a message. Direction is "out" or "in".
func Record(owner, direction, number, text string, segments int) *Message {
	if direction == "out" {
		route(owner, e164(number))
	}
	m := &Message{
		Direction: direction,
		Number:    e164(number),
		Text:      text,
		Segments:  segments,
		At:        time.Now(),
	}
	rec, err := userdb.Create(ns, owner, msgs, map[string]interface{}{
		"direction": m.Direction,
		"number":    m.Number,
		"text":      m.Text,
		"segments":  m.Segments,
		"at":        m.At.Format(time.RFC3339),
	}, false)
	if err != nil {
		app.Log("sms", "storing %s message for %s: %v", direction, owner, err)
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
		if f, ok := r.Data["segments"].(float64); ok {
			m.Segments = int(f)
		} else if n, ok := r.Data["segments"].(int); ok {
			m.Segments = n
		}
		if s, ok := r.Data["at"].(string); ok {
			m.At, _ = time.Parse(time.RFC3339, s)
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// route remembers who last texted a number, so a reply can be given back to
// them. It is written on every send.
//
// A separate record rather than a scan of the messages, because a message
// belongs to its owner and one account cannot read another's — which is the
// right rule, and leaves the router with nothing to search. This is the one
// fact the instance itself needs to know, so the instance owns it.
func route(owner, number string) {
	recs, err := userdb.List(ns, instance, routes, "mine",
		map[string]interface{}{"number": number}, "", "", 1)
	data := map[string]interface{}{
		"number": number, "owner": owner, "at": time.Now().Format(time.RFC3339),
	}
	if err == nil && len(recs) == 1 {
		userdb.Update(ns, instance, routes, recs[0].ID, data, false) //nolint:errcheck
		return
	}
	userdb.Create(ns, instance, routes, data, false) //nolint:errcheck
}

// OwnerOf finds which account an inbound message belongs to.
//
// One number serves the whole instance, so an arriving message has to be given
// to somebody. It goes to the account that most recently texted that number —
// the only defensible answer, because that is the conversation it is a reply
// to. A message from a number nobody here has texted belongs to nobody and is
// dropped rather than handed to whoever happens to be first in a list.
func OwnerOf(number string) string {
	number = e164(number)
	if number == "" {
		return ""
	}
	recs, err := userdb.List(ns, instance, routes, "mine",
		map[string]interface{}{"number": number}, "", "", 1)
	if err != nil || len(recs) == 0 {
		return ""
	}
	owner, _ := recs[0].Data["owner"].(string)
	return owner
}

// DeleteAll removes everything sms holds for an owner (account deletion).
//
// Opt-outs are not this owner's to delete: they belong to the number that asked
// to be left alone, and closing an account is not that number changing its mind.
func DeleteAll(owner string) {
	if owner == "" || owner == instance {
		return
	}
	if n, err := userdb.DeleteOwner(ns, owner); err != nil {
		app.Log("sms", "deleting %s's messages: %v", owner, err)
	} else if n > 0 {
		app.Log("sms", "deleted %d sms records for %s", n, owner)
	}
}
