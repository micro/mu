package server

// Every route this instance answers, registered once.
//
// This is the one function in the codebase with no coverage by construction: a
// route is not checked by anything until it is registered, and nothing
// registers them except the server starting. So a duplicate pattern builds,
// vets, and passes the whole suite — and then net/http panics at boot, systemd
// restarts into the same panic, and the instance is down with everything green
// behind it.
//
// That happened. /status has been the health page for a long time and the
// profile status setter was registered on top of it. Seventy-two restarts.
//
// Calling registerRoutes here is the whole test. It is not a re-derivation of
// what net/http considers a conflict — it is net/http, deciding, which is the
// only definition that matters.

import "testing"

func TestEveryRouteRegistersWithoutConflict(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering the routes panics, so this instance cannot boot:\n%v", r)
		}
	}()
	registerRoutes()
}
