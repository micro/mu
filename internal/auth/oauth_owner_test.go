package auth

import "testing"

// An OAuth client belongs to whoever registered it, and /token shows you yours.
//
// It showed everybody's. OAuthClient had no owner at all, the page called
// AllOAuthClients, and every signed-in person saw the names other people's MCP
// clients had registered under, their client ids and their dates — with a
// Delete beside each that worked on any of them.
func TestOAuthClientsAreOnlyVisibleToTheirOwner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mine := RegisterOAuthClient("ann", "Ann's laptop", nil)
	theirs := RegisterOAuthClient("bob", "Bob's laptop", nil)

	for _, c := range OAuthClientsFor("ann") {
		if c.ClientID == theirs.ClientID {
			t.Fatal("one account can see another account's OAuth client")
		}
	}

	var found bool
	for _, c := range OAuthClientsFor("ann") {
		if c.ClientID == mine.ClientID {
			found = true
		}
	}
	if !found {
		t.Error("an account cannot see its own OAuth client")
	}
}

// And cannot delete somebody else's.
func TestOneAccountCannotDeleteAnothersOAuthClient(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	theirs := RegisterOAuthClient("bob", "Bob's laptop", nil)

	if err := DeleteOAuthClient(theirs.ClientID, "ann"); err == nil {
		t.Fatal("a signed-in stranger deleted somebody else's OAuth client")
	}
	if GetOAuthClient(theirs.ClientID) == nil {
		t.Fatal("the client is gone even though the delete was refused")
	}

	if err := DeleteOAuthClient(theirs.ClientID, "bob"); err != nil {
		t.Error("the owner could not delete their own client")
	}
	if GetOAuthClient(theirs.ClientID) != nil {
		t.Error("the owner's delete did not remove it")
	}
}

// A client that registered itself belongs to nobody, so it is nobody's to see
// or to remove. Dynamic registration is anonymous by specification — there is no
// account to attribute it to, which is exactly why it must not land on a page
// headed "your" anything.
func TestASelfRegisteredClientBelongsToNobody(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	anon := RegisterOAuthClient("", "Some MCP client", nil)
	if anon.Account != "" {
		t.Fatal("anonymous registration recorded an owner")
	}

	for _, who := range []string{"ann", "bob"} {
		for _, c := range OAuthClientsFor(who) {
			if c.ClientID == anon.ClientID {
				t.Errorf("%s sees a client that registered itself", who)
			}
		}
		if err := DeleteOAuthClient(anon.ClientID, who); err == nil {
			t.Errorf("%s deleted a client that is not theirs", who)
		}
	}

	// The operator can still reach it, or it would be stranded on disk forever.
	if len(AllOAuthClients()) == 0 {
		t.Error("the operator's view lost it too")
	}
	ForceDeleteOAuthClient(anon.ClientID)
	if GetOAuthClient(anon.ClientID) != nil {
		t.Error("the operator could not remove it")
	}
}

// An empty account is not a wildcard. OAuthClientsFor("") returning everything
// would put the whole registry back on any page that happened to have no
// session — which is the same bug with a different spelling.
func TestNoAccountMatchesNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	RegisterOAuthClient("ann", "Ann's laptop", nil)

	if got := OAuthClientsFor(""); len(got) != 0 {
		t.Errorf("an empty account listed %d clients", len(got))
	}
	if err := DeleteOAuthClient("anything", ""); err == nil {
		t.Error("an empty account deleted a client")
	}
}
