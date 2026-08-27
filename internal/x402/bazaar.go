package x402

// The bazaar, and why there is not one.
//
// There was a discovery extension here. The x402 Bazaar is the ecosystem's
// answer to "a priced endpoint nobody can find is a shop with the lights off":
// facilitators index resources by reading an extension out of the 402 challenge
// itself, so you are listed at the moment a facilitator asks what you charge.
// It suited an MCP server exactly, because every field a listing wants — tool
// name, description, argument schema — is what tools/list already publishes.
//
// It was off by default and stayed off, which is the part worth reading twice.
// Listing tells a third party that this instance exists, what it sells and what
// it charges, and buys discovery by an index nobody is searching. There are no
// other Mu servers. Federation needs something to federate with, and when there
// is, DNS is how instances will find each other rather than a facilitator's
// catalogue — a Mu instance is already a domain with services under it, which
// is the same shape mail has used for forty years.
//
// So the setting is gone rather than defaulted off. A switch for something that
// is not going to happen is a decision left lying around for somebody to make
// by accident.
//
// The word is worth keeping in mind, though. A bazaar is a marketplace, and
// being one is a better ambition than being listed in somebody else's.
