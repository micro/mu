package inbox

// Writing to a person by name.
//
// The Write button on a profile puts @someone in the To box rather than their
// address, because /@somebody is a public page and the address was printed on
// it. The shortcut is the point and the address is not the sender's business,
// so the resolution happens here, on the way out.

import (
	"testing"

	"mu/internal/auth"
)

func TestAnAtNameResolvesToTheirAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "mu.test")

	if err := auth.Create(&auth.Account{ID: "henrik", Name: "Henrik", Secret: "s"}); err != nil {
		t.Fatalf("creating the account: %v", err)
	}

	got, ok := addressOfPerson("@henrik")
	if !ok {
		t.Fatal("@henrik resolved to nobody, so Write from their profile sends nothing")
	}
	if got != "henrik@mu.test" {
		t.Errorf("addressOfPerson(@henrik) = %q", got)
	}

	// Their address, not their agent's — a tag here would quietly run
	// somebody's agent instead of writing to them. service/mail draws that
	// line: untagged mail is just mail.
	if want := "henrik@mu.test"; got != want {
		t.Errorf("the resolved address carries a tag: %q", got)
	}

	// However it is written down.
	if got, ok := addressOfPerson("@HENRIK"); !ok || got != "henrik@mu.test" {
		t.Errorf("@HENRIK resolved to %q/%v — an account id is one name", got, ok)
	}

	// And a name nobody answers to is refused rather than turned into an
	// address that would bounce.
	if _, ok := addressOfPerson("@nobody-here"); ok {
		t.Error("a name with no account behind it produced an address")
	}
	if _, ok := addressOfPerson("@"); ok {
		t.Error("a bare @ produced an address")
	}
}
