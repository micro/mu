package agent

// What a run with no private context may reach.
//
// QueryOpts.Public is set for a mail or the web *group*: the
// conversation is not one person's, so mail, the wallet and the address book
// are not this conversation's to reach. The tool list for that case is
// publicToolsDesc, and this is the check behind it.
//
// It was called isGuestAllowedTool, because a signed-out visitor was the other
// caller with nothing private to reach. Signed-out runs are gone — every run
// now belongs to an account — so the group channel is the whole of it, and the
// name says so.
//
// The list of tools with no service behind them was written out here and again
// in agent/micro, both under a comment about the two hand-written allowlists
// they had replaced. There is one, next to the door that asks.

import "mu/internal/api"

func publicTool(name string) bool { return api.PublicTool(name) }
