package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// accountScoped names the services whose data belongs to a specific user, or
// that spend the caller's credits. Callers without an authenticated account —
// a guest agent, an app rendered for a logged-out visitor — must not reach
// them. This is the single definition; the agent and the app SDK both consult
// it so the two can never drift apart.
var accountScoped = map[string]bool{
	"mail":   true, // the user's own inbox
	"index":  true, // searches the caller's own mail alongside public content
	"images": true, // generation is metered against their wallet
	"events": true, // personal schedule
}

// AccountScoped reports whether a service requires an authenticated caller.
func AccountScoped(name string) bool { return accountScoped[strings.ToLower(name)] }

// Endpoints returns the RPC endpoint names a registered service exposes
// (e.g. "Server.Forecast"), sorted. It reads the live registry, so the set is
// whatever the service actually registered — never a hardcoded list.
func Endpoints(name string) ([]string, error) {
	ensure()
	svcs, err := reg.GetService(name)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range svcs {
		for _, e := range s.Endpoints {
			if e.Name == "" || seen[e.Name] {
				continue
			}
			// Skip go-micro's built-in debug endpoints; they are not domain
			// capabilities and should not appear as callable methods.
			if strings.HasPrefix(e.Name, "Debug.") {
				continue
			}
			seen[e.Name] = true
			out = append(out, e.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ResolveEndpoint maps a friendly method name to a registered endpoint. Callers
// name a method as "forecast" or "Forecast"; the registry knows it as
// "Server.Forecast". Matching is case-insensitive on the method half, and an
// exact "Handler.Method" is accepted as-is.
func ResolveEndpoint(svc, method string) (string, error) {
	eps, err := Endpoints(svc)
	if err != nil {
		return "", fmt.Errorf("unknown service %q", svc)
	}
	want := strings.ToLower(strings.TrimSpace(method))
	if want == "" {
		return "", fmt.Errorf("method is required")
	}
	for _, ep := range eps {
		if strings.ToLower(ep) == want {
			return ep, nil
		}
		if _, m, ok := strings.Cut(ep, "."); ok && strings.ToLower(m) == want {
			return ep, nil
		}
	}
	return "", fmt.Errorf("unknown method %q on service %q (available: %s)",
		method, svc, strings.Join(methodNames(eps), ", "))
}

// methodNames strips the handler prefix for display: Server.Forecast -> forecast.
func methodNames(eps []string) []string {
	out := make([]string, 0, len(eps))
	for _, ep := range eps {
		if _, m, ok := strings.Cut(ep, "."); ok {
			out = append(out, strings.ToLower(m))
			continue
		}
		out = append(out, strings.ToLower(ep))
	}
	return out
}

// CallDynamic invokes a service method known only by name, passing and
// returning untyped maps. This is what lets a caller that has no compile-time
// knowledge of a service — an app using the SDK, the agent dispatching a tool,
// a service calling another service — still use it.
//
// It is also the one place account identity is bound. Whatever the caller put
// in args["account_id"] is discarded and replaced with the authenticated
// account from the context, so no dynamic caller can scope a call to someone
// else by naming them. Callers set identity with WithAccount at the boundary
// where a session exists; everything downstream inherits it.
func CallDynamic(ctx context.Context, svc, method string, args map[string]any) (map[string]any, error) {
	ep, err := ResolveEndpoint(svc, method)
	if err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}

	// Identity comes from the context, never from the caller's arguments.
	// Handlers still read account_id off their request struct, so it is stamped
	// here rather than removed — but the value is Mu's, not the caller's.
	delete(args, "account_id")
	if acc := AccountFrom(ctx); acc != "" {
		args["account_id"] = acc
	}

	var rsp map[string]any
	if err := Call(ctx, svc, ep, &args, &rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}
