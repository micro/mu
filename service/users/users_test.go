package users

// What a directory entry has to contain.

import (
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/settings"
)

// The answer to "look somebody up" contains an address you can write to.
//
// users_find's own documentation says it is for turning "a name somebody
// mentioned into an address you can write to". It returned a username and a
// path, and left the caller to guess the domain — so an agent following that
// sentence needed something the answer did not contain.
func TestALookedUpUserCarriesTheirAddress(t *testing.T) {
	prev := settings.Get("MAIL_DOMAIN")
	t.Cleanup(func() { settings.Set("MAIL_DOMAIN", prev) })
	settings.Set("MAIL_DOMAIN", "example.test")

	got := addressOf("somebody")
	if got != "somebody@example.test" {
		t.Errorf("address = %q, want %q", got, "somebody@example.test")
	}
	if !strings.Contains(got, "@") {
		t.Error("an address with no at sign is not an address")
	}
}

// An instance with no mail domain publishes no address.
//
// Not name@localhost. This value is read by somebody deciding where to write,
// and a local address is a wrong answer in the shape of a right one — which is
// the judgement the users page already made in its lead sentence, and the
// opposite of the one mail.ConfiguredDomain makes for its own purposes.
func TestNoMailDomainMeansNoAddressRatherThanALocalOne(t *testing.T) {
	prev := settings.Get("MAIL_DOMAIN")
	t.Cleanup(func() { settings.Set("MAIL_DOMAIN", prev) })
	settings.Set("MAIL_DOMAIN", "")

	if got := addressOf("somebody"); got != "" {
		t.Errorf("address = %q on an instance with no mail domain, want empty", got)
	}
}

// The projection publishes it, not just the helper.
//
// publicOf is the one place a User is built, and a field that exists on the
// struct but is never filled is the same as no field — which is exactly the
// shape of bug this change is fixing, so testing addressOf alone would repeat
// it.
func TestTheProjectionFillsTheAddressIn(t *testing.T) {
	prev := settings.Get("MAIL_DOMAIN")
	t.Cleanup(func() { settings.Set("MAIL_DOMAIN", prev) })
	settings.Set("MAIL_DOMAIN", "example.test")

	u := publicOf(&auth.Account{ID: "somebody", Name: "Some Body"}, map[string]bool{})
	if u.Address != "somebody@example.test" {
		t.Errorf("Address = %q, want %q — the field is published empty however "+
			"well addressOf works", u.Address, "somebody@example.test")
	}
	// The username is still the local part of it, which is what ID promises.
	if !strings.HasPrefix(u.Address, u.ID+"@") {
		t.Errorf("%q is not %q at a domain", u.Address, u.ID)
	}
}
