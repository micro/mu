package apps

// What an edit says it did.
//
// The result was "Updated <name>." however many times it was called and
// whatever it changed. A run that edits an app ten times — which is what
// building one looks like — left ten identical lines in the transcript, which
// say the tool ran and nothing about what happened.

import (
	"strings"
	"testing"
)

// Two different edits read differently.
func TestAnEditSaysWhatItChanged(t *testing.T) {
	html := edited(&EditRequest{Slug: "x", HTML: strings.Repeat("a", 4300)})
	if !strings.Contains(html, "KB of HTML") {
		t.Errorf("an HTML edit reads %q and never says what was written", html)
	}

	meta := edited(&EditRequest{Slug: "x", Description: "A live scores board"})
	if !strings.Contains(meta, "description") {
		t.Errorf("a description edit reads %q", meta)
	}
	if html == meta {
		t.Errorf("two different edits read identically: %q", html)
	}

	// The size is the point of naming HTML separately: two rewrites of very
	// different sizes are different events, and a run that reports both as
	// "html" is back to a line that repeats.
	small := edited(&EditRequest{Slug: "x", HTML: "<p>hi</p>"})
	if small == html {
		t.Errorf("a nine-byte edit and a 4KB one read the same: %q", small)
	}
	if !strings.Contains(small, "bytes") {
		t.Errorf("a small edit reads %q; bytes are clearer than 0.0 KB", small)
	}
}

// A call that changes nothing says so.
//
// Worth more than "Updated": it is the shape of a loop about to run again for
// the same reason, and the model reads this too.
func TestAnEditThatChangesNothingSaysSo(t *testing.T) {
	got := edited(&EditRequest{Slug: "x"})
	if !strings.Contains(got, "nothing changed") {
		t.Errorf("an empty edit reads %q", got)
	}
}
