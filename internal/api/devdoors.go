package api

// The server supplies configured connection addresses. Keeping this dependency
// injected avoids coupling API rendering to the mail and SMS service packages.
//
// Two rows, and they belong on this page rather than on /contact. `mu ask "…"`
// and a curl invocation with a bearer token in it are answers to "how do I call
// this from a program", and the contact card answers "how do I write to it".
// The CLI row has pointed at /api since it was written, and until now this page
// said nothing about the CLI at all.

// A DevClient is one way a program reaches this instance's agent.
//
// Only the fields used by API documentation cross this boundary.
type DevClient struct {
	Address string // what you type or call
	Note    string // the one thing the address does not say
	Example string // the call itself, where an address is not self-explanatory
}

// DevClientsFunc reports them. Set by the server; nil on any build that has not
// wired it, in which case the section is simply not drawn.
var DevClientsFunc func() []DevClient

func devClients() []DevClient {
	if DevClientsFunc == nil {
		return nil
	}
	return DevClientsFunc()
}
