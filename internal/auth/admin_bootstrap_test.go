package auth

import (
	"os"
	"testing"
)

func TestShouldBootstrapAdmin(t *testing.T) {
	os.Unsetenv("ADMIN")
	os.Unsetenv("MU_ADMIN")

	// No ADMIN set: first account is admin, later ones are not.
	if !shouldBootstrapAdmin(&Account{ID: "alice"}, true) {
		t.Fatal("first account should be admin when ADMIN unset")
	}
	if shouldBootstrapAdmin(&Account{ID: "bob"}, false) {
		t.Fatal("non-first account should not be admin when ADMIN unset")
	}

	// ADMIN set: only listed identities are admin, regardless of order.
	os.Setenv("ADMIN", "alice@example.com, carol")
	defer os.Unsetenv("ADMIN")
	if !shouldBootstrapAdmin(&Account{ID: "u1", Email: "alice@example.com"}, false) {
		t.Fatal("email match should be admin")
	}
	if !shouldBootstrapAdmin(&Account{ID: "carol"}, false) {
		t.Fatal("username match should be admin")
	}
	if shouldBootstrapAdmin(&Account{ID: "dave"}, true) {
		t.Fatal("unlisted account must not be admin even if first, when ADMIN is set")
	}
}
