package service

// The gateway: one path every service call goes through.
//
// There were four, and which one a service used was historical accident. Some
// charged inside a function both doors happened to call (images, sms,
// whatsapp). Some charged in the page handler, so the same call was priced
// through the UI and free as a tool (places). Some were charged by a hardcoded
// list of paths in HTTP middleware, with the same result (blog, apps, stream).
// Most were charged nowhere at all — web_search, weather_forecast and every
// routes endpoint were declared with a price and debited by nothing.
//
// It was measurable from outside: on the live instance, two web_search calls
// against an ordinary account moved the balance from 75 to 75, while
// images_generate in the same minute moved it from 75 to 15. Same account, same
// protocol, same minute — the difference was which of the four paths the
// service had been written against.
//
// So Endpoint.Cost was a label. This is what makes it true: every call to a
// registered endpoint arrives here, and here is where the questions get asked.
// Does this operation have a price? Then it has an allowance, a balance check
// and a charge. Does it not? Then it passes straight through. There is no third
// answer and no per-service opinion about it.
//
// It is the go-micro handler wrapper, which is the framework's own seam for
// this — every tool call, agent step, CLI command and app SDK call reaches a
// service through service.Call, and that is an RPC, so it lands here. Pages are
// the one door that does not yet, because they reach past the endpoint into a
// service's package functions; moving them onto the endpoint is what finishes
// this, and until it does the services that charge for a page do it themselves.
//
// Everything cross-cutting belongs here for the same reason the charge does:
// rate limits, free allowances, moderation, audit. One wrapper is a thing you
// can change; twenty-two call sites are a thing you can forget.

import (
	"context"

	"go-micro.dev/v6/server"
)

// Gate is asked whether a call may proceed, and told when one has.
//
// Filled in at boot by internal/server, because the answer needs the wallet and
// the wallet sits above this. Unset — a build with no billing linked in — means
// every call passes, which is the same answer a self-hosted instance with no
// payments configured gives anyway.
var Gate struct {
	// Allow reports whether this account may perform this operation now, and
	// whether the call should be charged for when it succeeds. Called before
	// the handler runs.
	//
	// The two answers are separate because a call can be permitted and free: an
	// account inside its daily allowance is allowed through and billed nothing,
	// which is not the same as an operation that has no price.
	Allow func(account, operation string) (charge bool, err error)

	// Charge records that the operation happened. Called after the handler
	// returns without error, because nothing should be billed for a call that
	// failed.
	Charge func(account, operation string)
}

// gateway wraps every endpoint of one service.
//
// The Spec is captured rather than looked up per call: it is the thing that
// says what this service's methods cost, and it does not change after Register.
func gateway(spec Spec) server.HandlerWrapper {
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(ctx context.Context, req server.Request, rsp interface{}) error {
			op := spec.Operation(methodName(req.Method()))
			if op == "" || Gate.Allow == nil {
				// Free, or nothing here can charge. Straight through — and
				// deliberately without asking who is calling, because a free
				// call has no business demanding identity.
				return next(ctx, req, rsp)
			}

			// Who is paying. An empty account is an unauthenticated caller: the
			// door it came through decides whether that is allowed, and by the
			// time a priced call reaches here it has already been let in, so
			// there is nobody to bill and nothing to refuse.
			who := AccountFrom(ctx)
			if who == "" {
				return next(ctx, req, rsp)
			}

			charge, err := Gate.Allow(who, op)
			if err != nil {
				return err
			}
			if err := next(ctx, req, rsp); err != nil {
				return err
			}
			// Charged after the fact, so a call that failed is not billed. The
			// provider may still have cost us; charging for a failure loses an
			// account rather than a credit.
			if charge && Gate.Charge != nil {
				Gate.Charge(who, op)
			}
			return nil
		}
	}
}

// methodName turns go-micro's "Server.Search" into "Search".
//
// The handler type is the service's Server struct, so every endpoint arrives
// prefixed with it. Spec.Endpoints is keyed by the bare method name, which is
// also what the tool name derives from, so this is where the two meet.
func methodName(full string) string {
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '.' {
			return full[i+1:]
		}
	}
	return full
}

// Operation is the billable operation for one of this service's methods, or
// empty when the method is free.
func (s Spec) Operation(method string) string {
	if ep, ok := s.Endpoints[method]; ok {
		return ep.Cost
	}
	return ""
}
