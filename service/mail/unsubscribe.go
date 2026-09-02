package mail

// Turning the copies off, from inside one of them.
//
// # One click, no login
//
// An unsubscribe link that asks you to sign in first is not an unsubscribe
// link. Somebody who wants these to stop is, by definition, somebody who does
// not want to deal with this instance right now — and the mail arrived at an
// address only they read, which is the same proof of identity the link needs.
//
// So the token is an HMAC of the account id under a key this instance holds. It
// cannot be forged, it names exactly one account, and it does exactly one
// thing: stop the copies. It is not a session and it cannot read a mailbox.
//
// # Why not a stored random token
//
// Because there would be a second thing to keep, migrate and expire, one per
// account, and the only question it answers is one a signature answers with a
// single stored key. See forwardSecret for why that key is stored rather than
// made fresh each boot.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"html"
	"net/http"
	"strings"
	"sync"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

// forwardKey signs unsubscribe links.
//
// Persisted, unlike the CSRF key it is modelled on. That one is per-process on
// purpose: a token in a form is used within minutes and a restart invalidating
// it costs a retry. This one is in an email somebody may open in a week, and a
// link that says "this link is no longer valid" to somebody trying to stop
// receiving mail is the worst possible moment to ask them to try again.
var (
	forwardKeyOnce sync.Once
	forwardKey     []byte
)

func forwardSecret() []byte {
	forwardKeyOnce.Do(func() {
		if stored := strings.TrimSpace(settings.Get("MAIL_FORWARD_KEY")); stored != "" {
			if b, err := base64.RawURLEncoding.DecodeString(stored); err == nil && len(b) >= 32 {
				forwardKey = b
				return
			}
		}
		forwardKey = make([]byte, 32)
		if _, err := rand.Read(forwardKey); err != nil {
			return
		}
		settings.Set("MAIL_FORWARD_KEY", base64.RawURLEncoding.EncodeToString(forwardKey))
	})
	return forwardKey
}

// UnsubscribeToken is the proof that a link came from a mail we sent.
func UnsubscribeToken(accountID string) string {
	if accountID == "" {
		return ""
	}
	mac := hmac.New(sha256.New, forwardSecret())
	mac.Write([]byte("unsubscribe:" + accountID))
	return accountID + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:24]
}

// accountFromToken returns the account a token names, if the signature holds.
func accountFromToken(token string) string {
	i := strings.LastIndex(token, ".")
	if i <= 0 {
		return ""
	}
	id := token[:i]
	if !hmac.Equal([]byte(UnsubscribeToken(id)), []byte(token)) {
		return ""
	}
	return id
}

// forwardSetting is where the choice is kept, per account.
func forwardSetting(accountID string) string { return "mail_forward:" + accountID }

// ForwardingOn reports whether arriving mail is copied to this account's own
// address.
//
// Default on, and the default is what makes it worth having: every account that
// already exists has an inbox nobody told them about. The cost of being wrong
// is one unwanted mail with a one-click way out at the bottom of it.
func ForwardingOn(accountID string) bool {
	if accountID == "" {
		return false
	}
	return strings.TrimSpace(settings.Get(forwardSetting(accountID))) != "off"
}

// SetForwarding turns the copies on or off for one account.
func SetForwarding(accountID string, on bool) {
	if accountID == "" {
		return
	}
	if on {
		settings.Set(forwardSetting(accountID), "on")
		return
	}
	settings.Set(forwardSetting(accountID), "off")
}

// UnsubscribeHandler serves /mail/unsubscribe.
//
// GET shows what is about to happen and a button; POST does it. Not a GET that
// acts, because a mail client that prefetches links would unsubscribe people
// who never clicked — which is a real thing scanners and previewers do.
//
// The exception is a client's own List-Unsubscribe button, which sends
// POST directly with List-Unsubscribe=One-Click. That is handled by the same
// POST branch: it is a deliberate act by the recipient either way.
func UnsubscribeHandler(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("t"))
	if token == "" {
		token = strings.TrimSpace(r.PostFormValue("t"))
	}
	accountID := accountFromToken(token)
	if accountID == "" {
		app.NotFound(w, r, "That unsubscribe link is not valid.")
		return
	}

	if r.Method == http.MethodPost {
		SetForwarding(accountID, false)
		app.Respond(w, r, app.Response{
			Title: "Unsubscribed",
			HTML: `<div class="w-760"><h2>Done</h2>` +
				`<p>Mail sent to your Mu address will stay in your inbox and will not be ` +
				`copied to you by email.</p>` +
				`<p class="text-muted">You can turn it back on in your Account.</p>` +
				`<p><a href="/inbox">Go to your inbox &rarr;</a></p></div>`,
		})
		return
	}

	name := accountID
	if acc, err := auth.GetAccount(accountID); err == nil && acc != nil && acc.Email != "" {
		name = acc.Email
	}
	app.Respond(w, r, app.Response{
		Title: "Stop these emails",
		HTML: `<div class="w-760"><h2>Stop these emails?</h2>` +
			`<p>Mail sent to your Mu address is currently copied to ` +
			html.EscapeString(name) + `. Your inbox on this instance is not affected.</p>` +
			`<form method="POST" action="/mail/unsubscribe">` +
			`<input type="hidden" name="t" value="` + html.EscapeString(token) + `">` +
			`<button type="submit">Stop sending them</button></form></div>`,
	})
}
