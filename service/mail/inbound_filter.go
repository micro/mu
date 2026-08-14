// Who is allowed to send us mail.
//
// This instance does not accept mail from strangers. Five things get a message
// through, and a sender needs only one of them:
//
//  1. It is a reply to something we sent — In-Reply-To or References matches a
//     Message-ID we generated.
//  2. We have written to that address before, which is recorded on the way out.
//  3. The sender's domain is on the whitelist — see Whitelisted below for what
//     is on it and how to add to it.
//  4. The sender's address is verified on an account here. Somebody who proved
//     they own a mailbox is not a stranger, whatever their domain.
//  5. The message is addressed to support@ and nothing else. That one is
//     public on purpose; see supportIsPublic.
//
// Everything else is refused with a 550, so the sender's own server tells them
// rather than the message vanishing.
//
// The list used to say three, and had said three for a while after it became
// four. If you add a rule, say so here — this comment is the only place the
// policy is written down in one piece, and an operator deciding whether to
// whitelist a domain is reading it.
package mail

import (
	"strings"
	"sync"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/settings"
)

// ── Outbound message ID tracking ────────────────────────────
// We record every Message-ID we generate so we can recognise replies.

var (
	sentMu     sync.RWMutex
	sentMsgIDs = map[string]bool{} // Message-ID → true
	sentToAddr = map[string]bool{} // email addresses we've sent to
)

func init() {
	data.LoadJSON("mail_sent_ids.json", &sentMsgIDs)
	data.LoadJSON("mail_sent_to.json", &sentToAddr)

	var loaded map[string]bool
	if err := data.LoadJSON("mail_whitelist.json", &loaded); err == nil && loaded != nil {
		customWhitelist = loaded
	}
}

// RecordOutbound stores a sent message's ID and recipient so future
// replies and mail from that address are allowed through.
func RecordOutbound(messageID, toAddr string) {
	sentMu.Lock()
	defer sentMu.Unlock()

	if messageID != "" {
		sentMsgIDs[messageID] = true
		// Cap at 10k to prevent unbounded growth.
		if len(sentMsgIDs) > 10000 {
			i := 0
			for k := range sentMsgIDs {
				if i > 1000 {
					break
				}
				delete(sentMsgIDs, k)
				i++
			}
		}
	}
	if toAddr != "" {
		sentToAddr[strings.ToLower(toAddr)] = true
	}
	data.SaveJSON("mail_sent_ids.json", sentMsgIDs)
	data.SaveJSON("mail_sent_to.json", sentToAddr)
}

// isReplyToOurMail checks if In-Reply-To or References contain a
// Message-ID we generated.
func isReplyToOurMail(inReplyTo, references string) bool {
	sentMu.RLock()
	defer sentMu.RUnlock()

	for _, id := range extractMessageIDs(inReplyTo + " " + references) {
		if sentMsgIDs[id] {
			return true
		}
	}
	return false
}

// isSentToAddress returns true if we've previously sent mail to this
// address (auto-whitelisted on outbound).
func isSentToAddress(addr string) bool {
	sentMu.RLock()
	defer sentMu.RUnlock()
	return sentToAddr[strings.ToLower(addr)]
}

// extractMessageIDs pulls <...> bracketed IDs from a header value.
func extractMessageIDs(s string) []string {
	var ids []string
	for {
		start := strings.Index(s, "<")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], ">")
		if end < 0 {
			break
		}
		ids = append(ids, s[start:start+end+1])
		s = s[start+end+1:]
	}
	return ids
}

// ── Domain whitelist ────────────────────────────────────────
// Product/company domains whose automated mail is always allowed.
// Consumer addresses (gmail.com, outlook.com, etc.) are NOT here —
// those only get through if the user sent to them first.

var domainWhitelist = map[string]bool{
	// Google
	"google.com": true, "youtube.com": true, "googleapis.com": true,
	// Microsoft
	"microsoft.com": true, "outlook.com": false, "hotmail.com": false,
	"live.com": false, "microsoftonline.com": true, "azure.com": true,
	// Apple
	"apple.com": true, "icloud.com": false,
	// GitHub
	"github.com": true,
	// Amazon
	"amazon.com": true, "amazon.co.uk": true, "amazonaws.com": true,
	// Stripe
	"stripe.com": true,
	// Social
	"twitter.com": true, "x.com": true, "linkedin.com": true,
	"facebook.com": true, "instagram.com": true,
	// Dev tools
	"gitlab.com": true, "bitbucket.org": true, "atlassian.com": true,
	"notion.so": true, "slack.com": true, "zoom.us": true,
	"figma.com": true, "vercel.com": true, "netlify.com": true,
	"cloudflare.com": true, "digitalocean.com": true,
	"fly.io": true, "render.com": true, "railway.app": true,
	"heroku.com": true, "supabase.com": true, "firebase.google.com": true,
	// Payments / finance
	"paypal.com": true, "wise.com": true, "revolut.com": true,
	"monzo.com": true, "coinbase.com": true, "binance.com": true,
	// Shipping / commerce
	"royalmail.com": true, "dpd.co.uk": true, "ups.com": true,
	"fedex.com": true, "dhl.com": true, "ebay.com": true,
	"etsy.com": true, "shopify.com": true,
	// Comms
	"sendgrid.net": true, "mailchimp.com": true, "mailgun.com": true,
	"postmarkapp.com": true, "twilio.com": true,
	// UK services
	"gov.uk": true, "nhs.uk": true, "hmrc.gov.uk": true,
	// Security
	"letsencrypt.org": true, "cloudflare.net": true,
	// Email infrastructure (DMARC reports, etc.)
	"dmarc.yahoo.com": true,
	// Mu
	"micro.mu": true, "reminder.dev": true,
}

// Custom whitelist additions (persisted, managed by admin).
var (
	customWhitelistMu sync.RWMutex
	customWhitelist   = map[string]bool{}
)

// Whitelisted is the set of domains an operator has added, as a comma or space
// separated list: MAIL_WHITELIST="acme.com, partner.co.uk".
//
// It exists because the whitelist was unreachable. There is a
// mail_whitelist.json read at startup and nothing in the product ever wrote
// it — no page, no setting, no command — so "add a domain" meant knowing that
// file existed, finding it under ~/.mu/data, hand-editing JSON and restarting.
// That is not a policy an operator can hold. A setting is live-reloadable and
// appears at /admin/env beside everything else.
//
// The file still works: both are consulted, so an instance that already had one
// keeps it.
func Whitelisted() []string {
	raw := settings.Get("MAIL_WHITELIST")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	}) {
		if d := strings.ToLower(strings.TrimSpace(part)); d != "" {
			out = append(out, strings.TrimPrefix(d, "@"))
		}
	}
	return out
}

// isWhitelistedDomain checks both built-in and custom whitelists.
// Returns false for consumer email domains (gmail, outlook, etc.)
// even if they're in the map — those are explicitly set to false.
func isWhitelistedDomain(domain string) bool {
	domain = strings.ToLower(domain)

	// The operator's own list, from the setting.
	for _, d := range Whitelisted() {
		if d == domain {
			return true
		}
	}

	// Check custom whitelist first (admin-added).
	customWhitelistMu.RLock()
	if customWhitelist[domain] {
		customWhitelistMu.RUnlock()
		return true
	}
	customWhitelistMu.RUnlock()

	// Check built-in list. Entries set to false (gmail, outlook, etc.)
	// are explicitly NOT whitelisted.
	if allowed, exists := domainWhitelist[domain]; exists {
		return allowed
	}

	// Check parent domain for subdomains (e.g. mail.google.com → google.com).
	parts := strings.SplitN(domain, ".", 2)
	if len(parts) == 2 {
		if allowed, exists := domainWhitelist[parts[1]]; exists {
			return allowed
		}
	}

	return false
}

// CheckInboundAllowed decides whether an inbound email should be
// accepted. Returns ("", true) if allowed, or (reason, false) if
// it should be rejected.
func CheckInboundAllowed(fromAddr string, to []string, inReplyTo, references string) (string, bool) {
	// 0. Is it support mail? That address is public, so the whitelist does not
	// apply to it — the whole point of it is to hear from people this instance
	// has never heard of.
	if supportIsPublic(to) {
		if supportFlooding(fromAddr) {
			return "too many messages to support from this address today", false
		}
		return "", true
	}

	// 1. Is it a reply to something we sent?
	if inReplyTo != "" || references != "" {
		if isReplyToOurMail(inReplyTo, references) {
			return "", true
		}
	}

	// 2. Is the sender an address we've previously emailed?
	if isSentToAddress(fromAddr) {
		return "", true
	}

	// 3. Is the sender's domain whitelisted?
	parts := strings.Split(strings.ToLower(fromAddr), "@")
	if len(parts) == 2 && isWhitelistedDomain(parts[1]) {
		return "", true
	}

	// 4. Is the sender a verified email on an account here?
	//
	// Somebody who clicked a link in a mailbox to prove they own it is not a
	// stranger, and their own address should not need an operator to whitelist
	// its domain by hand. Without this, writing to your own agent from a
	// personal address only worked if that domain happened to be on a list
	// written for company mail.
	if VerifiedAccountAddress(fromAddr) {
		return "", true
	}

	return "sender not in whitelist and message is not a reply", false
}

// supportIsPublic reports whether a message is for support and only support.
//
// Only support: a sender who puts support@ alongside a user's address would
// otherwise use it as a skeleton key into every mailbox here. Addressed solely
// to support, the worst case is that an admin reads something they did not want
// to — which is what a support address is.
func supportIsPublic(to []string) bool {
	if len(to) == 0 {
		return false
	}
	for _, addr := range to {
		local := addr
		if i := strings.Index(local, "@"); i >= 0 {
			local = local[:i]
		}
		if !strings.EqualFold(strings.TrimSpace(local), SupportMailbox) {
			return false
		}
	}
	return true
}

// ── Support flood control ───────────────────────────────────
//
// The whitelist was doing two jobs: keeping spam out, and keeping strangers
// out. Support needs the first without the second, so it gets a cap instead —
// per sending address, which is the unit somebody has to keep making more of
// to get round it.
//
// Held in memory and lost on restart. That is the right trade for abuse
// control: a bot that has to wait for a mail server to be restarted is not
// getting much, and persisting it would mean a disk write per message to a
// mailbox that mostly receives nothing.

const (
	supportPerDay = 20
	supportWindow = 24 * time.Hour
)

var (
	supportMu   sync.Mutex
	supportSeen = map[string][]time.Time{} // sender → when they wrote
)

// supportFlooding records an attempt and reports whether it is over the cap.
func supportFlooding(fromAddr string) bool {
	key := strings.ToLower(strings.TrimSpace(fromAddr))
	if key == "" {
		return true // no address to hold responsible
	}

	supportMu.Lock()
	defer supportMu.Unlock()

	cutoff := time.Now().Add(-supportWindow)
	kept := supportSeen[key][:0]
	for _, t := range supportSeen[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	// Senders who stopped writing stop being remembered, so this cannot grow
	// without bound on an instance that has been up for months.
	if len(kept) == 0 {
		delete(supportSeen, key)
	} else {
		supportSeen[key] = kept
	}

	if len(kept) >= supportPerDay {
		return true
	}
	supportSeen[key] = append(supportSeen[key], time.Now())
	return false
}

// isOwnVerifiedAddress reports whether an inbound sender is the recipient's
// own verified email — you, writing to your own address or to one of your
// agents' aliases.
//
// This is the one relationship the instance can be certain about: the account
// holder proved they control that mailbox, by clicking a link in it or by
// sending back a code that arrived there. Mail from it to themselves is never
// spam and never needs whitelisting, and treating it like any other stranger is
// what made "email your agent" — the first thing anyone tries — fail silently
// into a folder.
//
// Any address the account has proved, not only the one it signs in with. That
// used to be the same thing, and the gap it left was the ordinary case: you
// sign up from a personal address and then write to your agent from work.
func isOwnVerifiedAddress(acc *auth.Account, fromAddr string) bool {
	return acc.Owns(fromAddr)
}

// SenderIsAccountOwner reports whether an address is the verified email of one
// particular account — the same question as isOwnVerifiedAddress, asked by
// account id rather than by record.
//
// Distinct from VerifiedAccountAddress below, which asks whether an address
// belongs to *anybody* here. That is the right question for letting mail in
// and the wrong one for letting a sender drive an agent: every account holder
// on the instance would qualify.
func SenderIsAccountOwner(ownerID, fromAddr string) bool {
	if ownerID == "" {
		return false
	}
	acc, err := auth.GetAccount(ownerID)
	if err != nil {
		return false
	}
	return isOwnVerifiedAddress(acc, fromAddr)
}

// AccountForVerifiedEmail finds the account that proved it owns an address.
//
// The shared agent mailbox has no owner in its own name — agent@<domain>
// belongs to the instance — so whose mail it is has to come from who sent it.
// A verified email is the only claim strong enough to answer that: the person
// proved they can read the mailbox. Nil when nobody has.
func AccountForVerifiedEmail(fromAddr string) *auth.Account {
	return auth.AccountForAddress(fromAddr)
}

// VerifiedAccountAddress reports whether an address is the verified email of
// any account on this instance. Used by the inbound whitelist: somebody who
// proved they own a mailbox is not a stranger, so their mail should reach this
// instance without an operator adding their domain by hand.
func VerifiedAccountAddress(fromAddr string) bool {
	return auth.AccountForAddress(fromAddr) != nil
}
