package mail

// A filtered message says what it is before it offers what to do about it.
//
// Not Spam and Delete were the first thing in the card, above the subject, so
// every row led with two buttons and you had to look past them to find out what
// they applied to. A screenful of filtered mail read as a column of controls
// with the messages interleaved.
//
// It is checked by reading the source because the card is built inside the mail
// handler, and standing up a session with spam in it to assert on the rendered
// page would test the fixture more than the order.

import (
	"strings"
	"testing"
)

func TestTheFilteredCardOffersItsActionsLast(t *testing.T) {
	src := readSource(t, "mail.go")

	// Bounded to the card's own template literal, found from the actions
	// outwards — so it locates the card whether the buttons are at the top or
	// the bottom, and no other view's markup can satisfy or break it.
	j := strings.Index(src, "action-footer")
	if j < 0 {
		t.Fatal("the filtered card no longer has a action-footer block")
	}
	start := strings.LastIndex(src[:j], "thread-preview card")
	end := strings.Index(src[j:], "`,")
	if start < 0 || end < 0 {
		t.Fatal("cannot bound the filtered card's template in mail.go")
	}
	card := src[start : j+end]

	subject := strings.Index(card, "mail-thread-subject")
	reason := strings.Index(card, "spam-info")
	actions := strings.Index(card, "action-footer")

	for name, at := range map[string]int{
		"mail-thread-subject": subject, "spam-info": reason, "action-footer": actions,
	} {
		if at < 0 {
			t.Fatalf("the filtered card no longer contains %s", name)
		}
	}

	if actions < subject {
		t.Error("the Not Spam and Delete buttons come before the subject, so every " +
			"row leads with two controls and you have to look past them to see " +
			"what they apply to")
	}
	if actions < reason {
		t.Error("the buttons come before the spam score and reason — that reason is " +
			"what the decision is made on, so it belongs above the decision")
	}
}

// And the buttons have a rule of their own.
//
// .action-footer had no CSS at all, which is why it simply stacked wherever it
// was written in the markup. Order alone would put them in the right place and
// still leave them crowding the line above.
func TestTheFilteredActionsAreStyledAsAFooter(t *testing.T) {
	css := readSource(t, "../../internal/app/html/mu.css")

	i := strings.Index(css, ".action-footer")
	if i < 0 {
		t.Fatal("`.action-footer` has no rule, so the buttons take whatever spacing " +
			"the surrounding markup happens to give them")
	}
	rule := css[i:]
	if end := strings.Index(rule, "}"); end > 0 {
		rule = rule[:end]
	}
	for _, want := range []string{"margin-top", "border-top"} {
		if !strings.Contains(rule, want) {
			t.Errorf("`.action-footer` sets no %s, so the buttons sit flush against "+
				"the message above them", want)
		}
	}
}
