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

// requestKey carries a caller's own id for one request, so a retry of it can be
// recognised as the same request rather than counted as a second one.
//
// It rides on metadata for the same reason identity does: it has to survive the
// RPC hop and it must not be an argument, because an argument is part of what
// the call means. A request id is not something the handler should ever see —
// it is addressed to the gateway.
const requestKey = "Mu-Request"

// WithRequest returns a context carrying id as this call's request id.
//
// Stamped by a door that has one: a JSON-RPC id, an Idempotency-Key header, a
// message id from a channel. A door with nothing to stamp leaves it alone, and
// the gateway falls back to recognising a retry by the request itself — see
// internal/service/dedupe.go.
func WithRequest(ctx context.Context, id string) context.Context {
	if id == "" {
		return metadata.Delete(ctx, requestKey)
	}
	return metadata.Set(ctx, requestKey, id)
}

// RequestFrom returns the caller's request id, or "" when there is none.
func RequestFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, ok := metadata.Get(ctx, requestKey)
	if !ok {
		return ""
	}
	return v
}

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
