package test

// A transcript says when.
//
// Every message has carried a Timestamp since there were messages, the socket
// sends it as `timestamp`, and displayRoomMessage dropped it — so a room was a
// wall of lines with no way to tell a reply from a minute ago from one from
// Tuesday. Reported as exactly that.
//
// Checked against mu.js rather than in a browser, because the failure is a
// field never read: nothing throws, nothing looks broken, the information is
// simply not on the screen. A test that renders the page and looks for a time
// would catch it, and so does this, at the point where it went missing.

import (
	"os"
	"strings"
	"testing"
)

func TestARoomMessageSaysWhenItWasSaid(t *testing.T) {
	b, err := os.ReadFile("../internal/app/html/mu.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)

	fn := js[strings.Index(js, "function displayRoomMessage"):]
	if i := strings.Index(fn, "\nfunction "); i > 0 {
		fn = fn[:i]
	}
	if fn == "" {
		t.Fatal("displayRoomMessage has gone, and it is what draws a message")
	}

	if !strings.Contains(fn, "msg.timestamp") {
		t.Error("displayRoomMessage never reads the timestamp the socket sends, " +
			"so a transcript has no times in it")
	}

	// data-timestamp, not a formatted string. updateTimestamps rewrites every
	// element carrying that attribute once a minute, so a relative label on a
	// page somebody leaves open does not sit there saying "1 min ago" for an
	// hour. Formatting it once here would look identical on load and be wrong
	// by the time anybody read it.
	if !strings.Contains(fn, "data-timestamp") {
		t.Error("the time is written once and never refreshed — updateTimestamps " +
			"only rewrites elements carrying data-timestamp")
	}
	if !strings.Contains(js, "function updateTimestamps") ||
		!strings.Contains(js, "[data-timestamp]") {
		t.Error("nothing refreshes data-timestamp any more, so the attribute is " +
			"doing nothing and the label goes stale")
	}

	// The wire carries RFC 3339 — Go marshals a time.Time to it — and timeAgo
	// takes unix seconds. Parsing is the step between, and skipping it gives
	// NaN, which renders as an empty span rather than as an error.
	if !strings.Contains(fn, "Date.parse(msg.timestamp)") {
		t.Error("the RFC 3339 timestamp is not parsed to unix seconds, which is " +
			"what timeAgo takes; unparsed it is NaN and draws nothing")
	}
}
