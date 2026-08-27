// Package notify is the door onto notifications: reaching a person when they
// are not looking at the page.
//
// # Why this is a service and internal/push is not
//
// internal/push argued that it was substrate — no page, no tools, no entry in
// the catalogue, the company of internal/x402 and internal/quota — on the
// grounds that "an agent has no business subscribing a browser to
// notifications". That is right about subscribing, and it was wrong about
// sending.
//
// Subscribing is a permission handshake between a browser and an account: three
// steps, every one refusable, none of them anything a caller chooses. It stays
// where it is, on /account beside the phone number, which is the other thing on
// this product you prove is yours.
//
// Sending is a capability, and it was the only transport that did not have one.
// Mail, SMS and chat each have a tool an agent can call; the interrupt channel —
// the one that reaches a person who is not looking — could only be used by three
// hardcoded call sites: mail arriving, a reminder firing, an admin alert. So
// "tell me if it drops below thirty" had nowhere to land. That is what this
// opens, and it is the whole reason ambient anything needs a service here.
//
// # The mechanism stays below
//
// VAPID, the ECDH, the aes128gcm framing and the subscription store are in
// internal/push and stay there, the way internal/x402 sits under the wallet.
// This package is a door, not a second implementation: it adds who may call,
// what it costs, and what the page shows.
//
// # It only reaches you
//
// Every entry point takes the caller's own account from the context and can
// address nothing else. There is no recipient argument, by design — a tool that
// took one would be a way to make somebody else's phone buzz, and the only
// bound on that would be whatever the agent was talked into.
package notify

import (
	"fmt"
	"strings"

	"mu/internal/push"
)

// bodyLimit is the same trim internal/push applies, stated here so a caller is
// told rather than silently shortened.
const bodyLimit = 300

// Send notifies one account's own devices.
//
// The error is the point. push.Send is fire-and-forget because mail delivery
// must not wait on a push service, but a caller that asked for this on purpose —
// an agent, a person pressing a button — is owed the answer, and "no device is
// registered" is the most common one.
func Send(account, title, body, url, from string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("a notification needs a title — it is the line on the lock screen")
	}
	if len(body) > bodyLimit {
		return fmt.Errorf("that body is %d characters; a notification shows about %d",
			len(body), bodyLimit)
	}
	if !push.Configured() {
		return fmt.Errorf("this instance cannot send notifications: it has no push keys")
	}
	if !push.Subscribed(account) {
		return fmt.Errorf("no device is registered — turn notifications on at /account first")
	}
	return push.SendNow(account, push.Notification{
		Title: title,
		Body:  strings.TrimSpace(body),
		URL:   strings.TrimSpace(url),
		From:  from,
		// No Tag. Tag collapses notifications about the same thing, and two
		// separate things an agent decided to say are not the same thing — the
		// second silently replacing the first is the one failure mode a person
		// cannot detect.
	})
}

// Devices is what this account can be reached on.
func Devices(account string) []push.Subscription { return push.Devices(account) }

// History is what has been sent to this account, newest first.
func History(account string, limit int) []push.Sent { return push.History(account, limit) }

// Reachable reports whether there is anywhere to send to at all.
func Reachable(account string) bool { return push.Configured() && push.Subscribed(account) }

// DeleteAll removes an account's devices and the record of what it was told,
// for account deletion.
//
// Registered from here rather than reaching past this package to internal/push,
// which is what the hook list used to do. The service is the door onto this
// data, so it is the thing that says what deleting an account means for it —
// and a scoped service that leaves its cleanup to somebody else's package is
// one the next person will not think to check.
func DeleteAll(account string) { push.DeleteAll(account) }
