package mail

import "testing"

// An address per agent, resolving to one account.
func TestSplitAlias(t *testing.T) {
	for local, want := range map[string][2]string{
		"asim":                {"asim", ""},
		"asim+research":       {"asim", "research"},
		"asim+research+daily": {"asim", "research+daily"},
		"asim+":               {"asim", ""},
		" asim+x ":            {"asim", "x"},
	} {
		account, tag := SplitAlias(local)
		if account != want[0] || tag != want[1] {
			t.Errorf("SplitAlias(%q) = (%q, %q), want (%q, %q)", local, account, tag, want[0], want[1])
		}
	}
}

// A leading plus has no account in front of it. It must not resolve to
// something — "+foo@" is not everybody's mail, it is a bad address.
func TestLeadingPlusIsNotAnAlias(t *testing.T) {
	account, tag := SplitAlias("+foo")
	if account != "+foo" || tag != "" {
		t.Errorf("a leading plus resolved to (%q, %q); it should stay whole and fail the account lookup", account, tag)
	}
}

// The tag goes into an address that has to survive every mail server between
// here and the sender, so anything needing quoting is dropped rather than
// escaped.
func TestAliasTagsAreSafeToType(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	for tag, want := range map[string]string{
		"research":       "asim+research@micro.mu",
		"Research":       "asim+research@micro.mu",
		"my receipts":    "asim+myreceipts@micro.mu",
		"a@b":            "asim+ab@micro.mu",
		`quote"inject`:   "asim+quoteinject@micro.mu",
		"drop-me_now.ok": "asim+drop-me_now.ok@micro.mu",
		"":               "asim@micro.mu",
		"!!!":            "asim@micro.mu",
	} {
		if got := AliasFor("asim", tag); got != want {
			t.Errorf("AliasFor(asim, %q) = %q, want %q", tag, got, want)
		}
	}
}

// The whole point: a tagged address is the same account's inbox.
func TestATaggedAddressBelongsToItsAccount(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	addr := AliasFor("asim", "research")
	local := addr[:len(addr)-len("@micro.mu")]
	account, tag := SplitAlias(local)
	if account != "asim" {
		t.Errorf("a tagged address does not resolve to its owner: %q -> %q", addr, account)
	}
	if tag != "research" {
		t.Errorf("the tag was lost in the round trip: %q", tag)
	}
}
