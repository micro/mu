package snapshot

// The plane carries more than cards.

import "testing"

// A service publishing two kinds of thing has two channels, and they do not
// read each other.
//
// The card is a page and the now is a fact. Keyed on the name alone they would
// be one key, and whichever published last would be what both readers got —
// which on a page is a paragraph of prompt text and in a prompt is a div.
func TestKindsDoNotShareAChannel(t *testing.T) {
	Channel("kindtest", "card").Publish("<div>a page</div>")
	Channel("kindtest", "now").Publish("Prices, as of now:")

	if got := Value("kindtest", "card"); got != "<div>a page</div>" {
		t.Errorf("the card channel holds %q", got)
	}
	if got := Value("kindtest", "now"); got != "Prices, as of now:" {
		t.Errorf("the now channel holds %q", got)
	}
}

// Asking twice gives the same channel, not a second subscription.
//
// Creating one subscribes to the broker, and both sides of the plane want a
// channel — the service that publishes and the agent that reads. A channel per
// caller would be two handlers doing identical work and two mirrors of one
// value.
func TestAChannelIsReused(t *testing.T) {
	if Channel("reuse", "now") != Channel("reuse", "now") {
		t.Error("a second call built a second channel, and with it a second subscription")
	}
}

// What was published is readable by the publisher, at once.
//
// The broker delivers to subscribers, and the publisher is one only by accident
// of being in the same process. Setting the mirror on publish is what makes a
// service able to read back what it just said — and what stops this breaking
// the day the producer and the reader are two processes.
func TestPublishingFillsTheMirror(t *testing.T) {
	Channel("mirror", "now").Publish("said")
	if got := Value("mirror", "now"); got != "said" {
		t.Errorf("the publisher cannot read back what it published: %q", got)
	}
}

// Nothing published is empty, not a stale answer from another name.
func TestAnUnwrittenChannelIsEmpty(t *testing.T) {
	if got := Value("never-written", "now"); got != "" {
		t.Errorf("an unwritten channel answered %q", got)
	}
}
