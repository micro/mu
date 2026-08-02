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
	"db":     true, // per-user records
	"wallet": true, // spends the caller's credits
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
// Identity is never taken from args. It rides on the context, set with
// WithAccount at the boundary where a session exists, and everything downstream
// inherits it — so no dynamic caller can scope a call to someone else by naming
// them in the arguments.
func CallDynamic(ctx context.Context, svc, method string, args map[string]any) (map[string]any, error) {
	ep, err := ResolveEndpoint(svc, method)
	if err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}

	// Handlers read identity from the context alone, so an account_id in the
	// arguments is meaningless. Strip it anyway: it must not reach a handler
	// that might one day be tempted to read it.
	delete(args, "account_id")

	var rsp map[string]any
	if err := Call(ctx, svc, ep, &args, &rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}
