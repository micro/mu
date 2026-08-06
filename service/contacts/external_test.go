package contacts

import (
	"strings"
	"testing"
)

func withExternal(t *testing.T, connected bool, people []External) {
	t.Helper()
	pc, pf := ExternalConnected, ExternalFind
	ExternalConnected = func(string) bool { return connected }
	ExternalFind = func(string, string) []External { return people }
	t.Cleanup(func() { ExternalConnected, ExternalFind = pc, pf })
}

// The gap this service was created to close, still open: contacts could only
// resolve names somebody had typed into it, and almost nobody types their
// address book in twice. "Email Sarah about Thursday" failed because Mu had
// never heard of Sarah.
func TestANameResolvesFromTheBookYouAlreadyHave(t *testing.T) {
	withExternal(t, true, []External{{Name: "Sarah Chen", Email: "sarah@example.com"}})

	own, ext := FindEverywhere("nobody", "sarah")
	if len(own) != 0 {
		t.Fatalf("an empty Mu address book returned %d matches", len(own))
	}
	if len(ext) != 1 || ext[0].Email != "sarah@example.com" {
		t.Fatalf("the attached book did not resolve the name: %+v", ext)
	}
	if ext[0].Source != ExternalName {
		t.Errorf("the match does not say where it came from: %+v", ext[0])
	}
}

// Somebody who wrote an address down here meant that address. An auto-collected
// duplicate from the other book must not appear beside it as a second option.
func TestTheCuratedBookWinsAndDuplicatesAreDropped(t *testing.T) {
	owner := "curator"
	if _, err := Add(owner, "Sarah Chen", "sarah@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, c := range List(owner) {
			_ = Remove(owner, c.ID)
		}
	})

	// The same person, and a different one, from the attached book.
	withExternal(t, true, []External{
		{Name: "Sarah Chen", Email: "SARAH@example.com"},
		{Name: "Sarah Okonjo", Email: "okonjo@example.com"},
	})

	own, ext := FindEverywhere(owner, "sarah")
	if len(own) != 1 {
		t.Fatalf("the curated match was lost: %+v", own)
	}
	for _, p := range ext {
		if strings.EqualFold(p.Email, "sarah@example.com") {
			t.Error("the same address was offered twice, once from each book")
		}
	}
	if len(ext) != 1 || ext[0].Email != "okonjo@example.com" {
		t.Errorf("the genuinely different person was dropped: %+v", ext)
	}
}

// Nothing attached: exactly the behaviour from before any of this existed.
func TestWithNothingAttachedOnlyTheOwnBookIsSearched(t *testing.T) {
	pc, pf := ExternalConnected, ExternalFind
	ExternalConnected, ExternalFind = nil, nil
	t.Cleanup(func() { ExternalConnected, ExternalFind = pc, pf })

	own, ext := FindEverywhere("nobody", "sarah")
	if len(ext) != 0 {
		t.Errorf("an instance with no client returned outside matches: %+v", ext)
	}
	if own == nil && len(own) != 0 {
		t.Error("the own-book search broke")
	}
}

// An outside match has no id, because Mu holds no record to refer back to.
// Offering one would be offering something that fails.
func TestOutsideMatchesCarryNoIdToDeleteBy(t *testing.T) {
	out := RenderExternal([]External{{Name: "Sarah Chen", Email: "sarah@example.com", Source: "Google Contacts"}})

	if strings.Contains(out, "id:") {
		t.Errorf("an outside match offered an id: %s", out)
	}
	for _, want := range []string{"Sarah Chen", "sarah@example.com", "Google Contacts"} {
		if !strings.Contains(out, want) {
			t.Errorf("the rendered match is missing %q: %s", want, out)
		}
	}
}

// The ask appears where it is earned, and never once it has been answered.
func TestTheConnectHintAppearsOnlyWhenItWouldHelp(t *testing.T) {
	ExternalConnected = nil
	if got := connectHint("someone"); got != "" {
		t.Errorf("an instance with no client offered to connect one: %q", got)
	}

	withExternal(t, false, nil)
	if got := connectHint("someone"); !strings.Contains(got, "/contacts") {
		t.Errorf("someone with nothing attached was not told they could: %q", got)
	}

	withExternal(t, true, nil)
	if got := connectHint("someone"); got != "" {
		t.Errorf("someone who already connected was asked again: %q", got)
	}
}

// The card asks, and points at the account page for withdrawing — which is one
// action covering everything, so it must not be duplicated here.
func TestTheContactsCardAsksButDoesNotOfferDisconnect(t *testing.T) {
	ExternalConnected = nil
	if got := connectCard("someone", ""); got != "" {
		t.Errorf("an instance with no client rendered a connect card: %q", got)
	}

	withExternal(t, false, nil)
	card := connectCard("someone", "")
	if !strings.Contains(card, "/oauth2/google/contacts") {
		t.Errorf("the card does not lead anywhere:\n%s", card)
	}
	if !strings.Contains(card, "nothing is copied") {
		t.Errorf("the card does not say that nothing is imported:\n%s", card)
	}

	withExternal(t, true, nil)
	connected := connectCard("someone", "connected")
	if strings.Contains(connected, "Disconnect") {
		t.Errorf("withdrawing access was duplicated onto the contacts page:\n%s", connected)
	}
	if !strings.Contains(connected, `href="/account"`) {
		t.Errorf("the card does not say where to manage it:\n%s", connected)
	}
}
