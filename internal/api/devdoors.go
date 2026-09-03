package api

// The doors that are not MCP, handed down from the product.
//
// mu/client holds every way in and this package may not see it: internal/ is
// underneath the product, and a package down here that imports up there has
// inverted the layering — see test.TestInternalNeverImportsTheProduct. The
// server assembles both halves and knows both, so it hands this one over, the
// same way app.ToolCountFunc goes the other way.
//
// Two rows, and they belong on this page rather than on /contact. `mu ask "…"`
// and a curl invocation with a bearer token in it are answers to "how do I call
// this from a program", and the contact card answers "how do I write to it".
// The CLI row has pointed at /api since it was written, and until now this page
// said nothing about the CLI at all.

// A DevClient is one way a program reaches this instance's agent.
//
// A copy of the fields this page draws rather than the product's own type,
// because taking the type would be taking the import. Three strings is a
// smaller thing to keep in step than a package boundary is to give up.
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
