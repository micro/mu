package service

import (
	"context"

	"go-micro.dev/v6/metadata"
)

// accountKey is the call-metadata key carrying the authenticated account.
//
// Identity used to travel as an "account_id" argument, which meant it was only
// as trustworthy as the caller. Two boundaries independently scrubbed it — the
// app SDK over HTTP, and the agent's tool wrapper — and both were correct, but
// the rule lived in two places because the argument itself guaranteed nothing.
// A third caller (a service calling another service) would have needed a third
// copy, and the failure mode of forgetting is silent cross-account access to
// mail, index, images and events.
//
// Metadata rides on the context instead. A caller cannot set it by choosing
// what to put in a request body, and it is stamped once at the boundary where a
// real session exists.
const accountKey = "Mu-Account"

// WithAccount returns a context carrying accountID as the authenticated caller.
// Passing an empty id clears any inherited identity rather than leaving it in
// place, so a guest request can never borrow the previous caller's account.
func WithAccount(ctx context.Context, accountID string) context.Context {
	if accountID == "" {
		return metadata.Delete(ctx, accountKey)
	}
	return metadata.Set(ctx, accountKey, accountID)
}

// AccountFrom returns the authenticated account on ctx, or "" for a guest.
// Handlers should prefer this over any account field on the request: this value
// was set by Mu, the request field was set by whoever made the call.
func AccountFrom(ctx context.Context) string {
	v, ok := metadata.Get(ctx, accountKey)
	if !ok {
		return ""
	}
	return v
}
