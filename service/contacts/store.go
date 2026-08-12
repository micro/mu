package contacts

// The address book itself is internal/contacts. This package is the service
// over it: the tools, the page, and the Google People bridge.
//
// The names below are re-exported rather than replaced at every call site.
// service/sms wanted one function — is this number in your address book — and
// importing a whole service to get it is what that cost; the fix is where the
// store lives, not what this package is called from inside it.

import store "mu/internal/contacts"

// Contact is one person in the caller's address book.
type Contact = store.Contact

var (
	Add       = store.Add
	Find      = store.Find
	HasEmail  = store.HasEmail
	List      = store.List
	Remove    = store.Remove
	Render    = store.Render
	DeleteAll = store.DeleteAll
)
