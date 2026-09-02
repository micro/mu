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

// agentKey says this call is a tool the model chose to run, rather than
// something a person did.
//
// It rides on metadata for the reasons above: it must survive the RPC hop, and
// it must not be an argument, because an argument is set by the caller and this
// is a fact about *who the caller is*. The agent's tool wrapper stamps it —
// see injectAccount, which is the one place every tool call passes through.
//
// It exists because "do this" and "write down that this should be done" are the
// same call with different consequences depending on who made it. A person
// filing a task for the agent means start it. An agent filing a task for itself
// while it is already running means record it — starting a second run for the
// same request is how one question becomes a loop, and the guard has to be able
// to tell the two apart. See service/tasks.
const agentKey = "Mu-Agent-Run"

// WithAgentRun marks ctx as a tool call inside an agent's own run.
func WithAgentRun(ctx context.Context) context.Context {
	return metadata.Set(ctx, agentKey, "1")
}

// InAgentRun reports whether this call is the model reaching for a tool.
//
// False for a person pressing a button, a REST call, or an agent's own
// scheduled work reaching a service directly — all of which are somebody
// deciding, which is the distinction that matters.
func InAgentRun(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, ok := metadata.Get(ctx, agentKey)
	return ok && v == "1"
}
