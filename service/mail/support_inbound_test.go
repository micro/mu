package mail

// Support has to be reachable by people this instance has never heard of.
//
// That is the entire job of the address, and it was the one thing the inbound
// filter would not allow: the filter saw only the sender, so a stranger writing
// to support@ was refused exactly like a stranger writing to anybody. Somebody
// whose payment failed before they verified an address wrote once and got a
// 550 back.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/settings"
)

func resetSupportRate(t *testing.T) {
	t.Helper()
	supportMu.Lock()
	supportSeen = map[string][]time.Time{}
	supportMu.Unlock()
}

func TestAStrangerCanReachSupport(t *testing.T) {
	resetSupportRate(t)

	for _, from := range []string{
		"stranger@gmail.com", "customer@outlook.com", "someone@randomcompany.co.uk",
	} {
		if _, ok := CheckInboundAllowed(from, []string{"support@micro.mu"}, "", ""); !ok {
			t.Errorf("%s cannot reach support, which is who support is for", from)
		}
		// And is still a stranger everywhere else.
		if _, ok := CheckInboundAllowed(from, []string{"asim@micro.mu"}, "", ""); ok {
			t.Errorf("%s got into a user's mailbox", from)
		}
	}
}

func TestSupportIsNotASkeletonKey(t *testing.T) {
	resetSupportRate(t)

	// support@ alongside a real mailbox must not open the real mailbox.
	if _, ok := CheckInboundAllowed("spammer@gmail.com",
		[]string{"support@micro.mu", "asim@micro.mu"}, "", ""); ok {
		t.Error("adding support@ to the recipients let a stranger into a user's mailbox")
	}

	// Several recipients, all support: still fine.
	if _, ok := CheckInboundAllowed("stranger@gmail.com",
		[]string{"support@micro.mu", "SUPPORT@micro.mu"}, "", ""); !ok {
		t.Error("two copies of the support address were refused")
	}
}

func TestSupportCanBeFloodedOnlySoFar(t *testing.T) {
	resetSupportRate(t)

	const from = "flood@gmail.com"
	to := []string{"support@micro.mu"}
	for i := 0; i < supportPerDay; i++ {
		if _, ok := CheckInboundAllowed(from, to, "", ""); !ok {
			t.Fatalf("refused message %d, under the cap of %d", i+1, supportPerDay)
		}
	}
	reason, ok := CheckInboundAllowed(from, to, "", "")
	if ok {
		t.Fatalf("accepted message %d, over the cap", supportPerDay+1)
	}
	if !strings.Contains(reason, "too many") {
		t.Errorf("unhelpful refusal: %q", reason)
	}

	// One sender being capped does not cap anybody else.
	if _, ok := CheckInboundAllowed("someone-else@gmail.com", to, "", ""); !ok {
		t.Error("one flooder locked everybody out of support")
	}
}

func TestAnAddresslessSenderCannotReachSupport(t *testing.T) {
	resetSupportRate(t)

	if _, ok := CheckInboundAllowed("", []string{"support@micro.mu"}, "", ""); ok {
		t.Error("a message with no sender got through, so there is nobody to rate limit")
	}
}

func TestTheOperatorsWhitelistIsReadFromTheSetting(t *testing.T) {
	prev := settings.Get("MAIL_WHITELIST")
	settings.Set("MAIL_WHITELIST", "acme.com, @partner.co.uk\nthird.example")
	t.Cleanup(func() { settings.Set("MAIL_WHITELIST", prev) })

	got := Whitelisted()
	want := map[string]bool{"acme.com": true, "partner.co.uk": true, "third.example": true}
	if len(got) != len(want) {
		t.Fatalf("parsed %v, want %d domains", got, len(want))
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("parsed %q, which was not asked for", d)
		}
	}

	// And it actually lets that domain in.
	resetSupportRate(t)
	if _, ok := CheckInboundAllowed("someone@acme.com", []string{"asim@micro.mu"}, "", ""); !ok {
		t.Error("a domain on the operator's whitelist was still refused")
	}
	if _, ok := CheckInboundAllowed("someone@notacme.com", []string{"asim@micro.mu"}, "", ""); ok {
		t.Error("the whitelist matched a domain that merely resembles one on it")
	}
}

func TestTheHeaderCommentStillListsEveryRule(t *testing.T) {
	// The comment said "three kinds" for a while after there were four. It is
	// the only place the policy is written down in one piece, and an operator
	// deciding whether to whitelist a domain is reading it.
	src := readSource(t, "inbound_filter.go")
	head := src
	if i := strings.Index(src, "package mail"); i > 0 {
		head = src[:i]
	}
	for _, want := range []string{
		"reply", "written to that address before", "whitelist",
		"verified on an account", "support@",
	} {
		if !strings.Contains(strings.ToLower(head), strings.ToLower(want)) {
			t.Errorf("the policy comment no longer mentions %q", want)
		}
	}
	if strings.Contains(head, "three kinds") {
		t.Error("the comment still says three kinds")
	}
}
