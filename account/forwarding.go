package account

// Whether mail arriving here is copied to the address you signed up with.
//
// The setting itself is service/mail's — it is a fact about mail, the
// unsubscribe link in every forwarded message writes it, and the forwarder
// reads it. These two are the account page's door to it.
//
// Hooks rather than an import, because account is a product package and
// service/mail is a service: internal/server wires the two, the same way it
// wires everything else here. It also means /account renders correctly on an
// instance built without the mail service rather than failing to compile.
var (
	// MailForwarding reports whether copies are on for an account.
	MailForwarding func(accountID string) bool
	// SetMailForward turns them on or off.
	SetMailForward func(accountID string, on bool)
)

// MailForwardingOn is MailForwarding with a default.
//
// Off when nothing is wired, because a page that cannot ask must not claim
// mail is being sent somewhere.
func MailForwardingOn(accountID string) bool {
	if MailForwarding == nil {
		return false
	}
	return MailForwarding(accountID)
}

// SetMailForwarding is SetMailForward, and does nothing when unwired.
func SetMailForwarding(accountID string, on bool) {
	if SetMailForward != nil {
		SetMailForward(accountID, on)
	}
}
