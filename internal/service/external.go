package service

import (
	"errors"
	"fmt"
	"sort"
)

// This file is the seam for services that live outside this repository —
// micro/xyz and anything like it.
//
// Two facts force the shape, and neither is a preference:
//
//   - Go's internal/ rule means an external module cannot import
//     mu/internal/service, so an outside service cannot call Register,
//     AccountFrom or Call itself.
//   - This module is named "mu", not a fetchable path, so an external module
//     cannot import anything here even if it were public.
//
// So the dependency can only run one way: Mu imports the external module and
// does the wiring. An external service depends on go-micro and nothing else,
// which is exactly the rule we wanted — enforced by the build rather than by
// discipline.
//
// What an external module exports is a plain map of service name to handler:
//
//	package xyz
//
//	func Services(c client.Client) map[string]any {
//	    return map[string]any{
//	        "convert": convert.New(c),
//	    }
//	}
//
// and Mu registers them all in one call. Adding a service to xyz needs no
// change here.

// AccountKey is the call-metadata key carrying the authenticated account.
//
// External services cannot import this package, so they read it straight from
// go-micro metadata:
//
//	account, _ := metadata.Get(ctx, service.AccountKey)  // "Mu-Account"
//
// That makes this string a published contract. It must not change.
const AccountKey = accountKey

// Client() already exists in service.go and is what Mu hands to an external
// module so its services can call Mu's services by name — the only capability
// an outside service needs that go-micro does not give it directly.

// RegisterAll registers a set of externally-defined services, keyed by name.
// Registration is attempted for every entry so one bad handler does not hide
// the rest; the errors are joined.
//
// Names are registered in sorted order purely so startup logs and any failure
// read the same way twice.
func RegisterAll(svcs map[string]any) error {
	names := make([]string, 0, len(svcs))
	for name := range svcs {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		h := svcs[name]
		if h == nil {
			errs = append(errs, fmt.Errorf("service %q has no handler", name))
			continue
		}
		if err := Register(Spec{Name: name, Handler: h}); err != nil {
			errs = append(errs, fmt.Errorf("service %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
