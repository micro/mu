package service

// What a call cost this instance, as opposed to what it was priced at.
//
// Every price in quota.json is the marginal cost of an operation: a model call,
// or a third party billed per request. The gateway charges that price on every
// call — and a great many calls do not incur it. service/weather keeps one
// forecast per place per half hour and serves it to everybody; service/tiles
// fetches a map tile once, ever. On a hit the instance pays nothing and the
// caller was charged anyway, so the saving went to the operator and the person
// who asked second paid the same as the person who asked first.
//
// That is backwards for a shared instance. The reason to call a tool here
// rather than hold your own API key is that somebody else may already have
// asked — and if that is true, it should be true in the bill.
//
// So: the price is what a *fetch* costs, and a call that did not fetch is not
// charged for. A handler that answered from something it already had says so,
// and the gateway settles accordingly.
//
// Reserved first, settled after. The balance check still happens before the
// handler runs, so a caller who cannot afford the worst case is refused rather
// than discovering it halfway; what changes is only whether the charge lands.
// Same shape as the payment rail's Verify-then-Settle, for the same reason.
//
// The default is to charge. A handler that says nothing is billed exactly as it
// was before, so forgetting this costs a caller a credit and never costs the
// instance its revenue — the failure that is noticed rather than the one that
// is not.

import (
	"context"
	"sync/atomic"
)

// meter is what one call reports about itself. Carried by pointer in the
// context so a handler can write to it while the gateway holds the read.
//
// atomic because a handler may fan out: two goroutines answering parts of one
// call, either of which may have hit a cache.
type meter struct{ free atomic.Bool }

type meterKey struct{}

// withMeter attaches a fresh meter to a call.
func withMeter(ctx context.Context) (context.Context, *meter) {
	m := &meter{}
	return context.WithValue(ctx, meterKey{}, m), m
}

// ServedFromCache says this answer cost the instance nothing to produce, so the
// caller is not charged for it.
//
// The name is what happened rather than what to bill, because that is the
// service's business and billing is not: a handler knows whether it went and
// fetched something, and it has no opinion about credits. Anything that did not
// reach a paid provider counts — a cache hit, a stored file, a keyless fallback.
//
// A no-op when there is no meter in the context, which is every path that does
// not go through the gateway: a page, a background loop, a test. Those are not
// charged either, so there is nothing to tell.
//
// Say it only when it is true. A handler that reports this for a call that did
// reach the provider is giving the answer away.
func ServedFromCache(ctx context.Context) {
	if m, ok := ctx.Value(meterKey{}).(*meter); ok && m != nil {
		m.free.Store(true)
	}
}

// servedFromCache reports what the handler said.
func (m *meter) servedFromCache() bool { return m != nil && m.free.Load() }
