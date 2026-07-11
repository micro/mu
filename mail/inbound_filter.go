// Strict inbound mail filter. Only three kinds of mail get through:
//
//  1. Replies to messages we sent (In-Reply-To or References match a
//     Message-ID we generated).
//  2. Mail from whitelisted domains (product/company mail, not
//     consumer addresses like @gmail.com).
//  3. Mail from addresses we've previously sent to (auto-whitelisted
//     on outbound).
//
// Everything else is silently rejected at the SMTP level.
package mail

import (
	"strings"
	"sync"

	"mu/internal/data"
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

// WhitelistDomain adds a domain to the custom whitelist.
func WhitelistDomain(domain string) {
	customWhitelistMu.Lock()
	defer customWhitelistMu.Unlock()
	customWhitelist[strings.ToLower(domain)] = true
	data.SaveJSON("mail_whitelist.json", customWhitelist)
}

// UnwhitelistDomain removes a domain from the custom whitelist.
func UnwhitelistDomain(domain string) {
	customWhitelistMu.Lock()
	defer customWhitelistMu.Unlock()
	delete(customWhitelist, strings.ToLower(domain))
	data.SaveJSON("mail_whitelist.json", customWhitelist)
}

// ListWhitelistedDomains returns all custom-whitelisted domains.
func ListWhitelistedDomains() []string {
	customWhitelistMu.RLock()
	defer customWhitelistMu.RUnlock()
	var result []string
	for d := range customWhitelist {
		result = append(result, d)
	}
	return result
}

// isWhitelistedDomain checks both built-in and custom whitelists.
// Returns false for consumer email domains (gmail, outlook, etc.)
// even if they're in the map — those are explicitly set to false.
func isWhitelistedDomain(domain string) bool {
	domain = strings.ToLower(domain)

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
func CheckInboundAllowed(fromAddr, inReplyTo, references string) (string, bool) {
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

	return "sender not in whitelist and message is not a reply", false
}

// SaveSentIDs is a no-op helper to force a save (called on graceful shutdown).
func SaveSentIDs() {
	sentMu.RLock()
	defer sentMu.RUnlock()
	data.SaveJSON("mail_sent_ids.json", sentMsgIDs)
	data.SaveJSON("mail_sent_to.json", sentToAddr)
}
