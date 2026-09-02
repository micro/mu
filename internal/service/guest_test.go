package service

// What a signed-out caller can reach.
//
// It was "everything that is not account-scoped", which is two dozen services
// and all their methods in the prompt of every stranger's question. The set is
// small and named now, and these are the two properties it must keep: nothing
// personal in it, and a service this instance does not run is not offered.

import "testing"

func TestGuestNeverReachesAnAccount(t *testing.T) {
	// The registry is what it is in this build; whatever is in it, none of it
	// may be scoped. This is the invariant that matters, and it holds for an
	// empty registry too.
	for _, name := range Guest() {
		if AccountScoped(name) {
			t.Errorf("%s is account-scoped and is offered to signed-out callers", name)
		}
	}
}

// And the list is a preference, not a claim. An instance that does not run one
// of these offers fewer doors rather than a broken one.
func TestGuestOnlyNamesServicesThatExist(t *testing.T) {
	for _, name := range Guest() {
		if _, ok := SpecFor(name); !ok {
			t.Errorf("%s is offered to guests and is not registered here", name)
		}
	}
}

// Small on purpose: the fastest thing you can do for a tool-calling loop is
// give it fewer tools to consider, and a guest run is meant to be quick.
func TestTheGuestSetStaysSmall(t *testing.T) {
	if n := len(guestOrder); n > 10 {
		t.Errorf("the guest set names %d services. It is meant to be a handful — "+
			"the reason it exists is that the alternative was two dozen in every "+
			"stranger's prompt", n)
	}
	seen := map[string]bool{}
	for _, name := range guestOrder {
		if seen[name] {
			t.Errorf("%s is named twice", name)
		}
		seen[name] = true
	}
}
