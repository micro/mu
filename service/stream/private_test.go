package stream

// The timeline is public. /stream serves it with no session and stream_list
// answers an unauthenticated MCP caller, which is deliberate — it is the
// instance's own timeline, the sign that something is running here.
//
// That makes every entry on it a publication. Three call sites forgot: a fired
// reminder posted its title, a standing instruction posted its title, and
// inbound mail posted the sender and the subject. A reminder somebody had
// written down privately — "Dentist about the biopsy results" — went to the
// open internet the moment it fired, along with the account id of whoever
// owned it.
//
// The old fix was to guard the *metadata*: whatever gets posted, do not let a
// public entry name whose it was. That was a guard against the symptom. There
// was no way to say "this one is theirs", so anything personal reaching the
// timeline was a leak by construction, and the only defence was every call
// site remembering.
//
// Entry.Account is the fix: whoever announces a fact says at that moment
// whether it belongs to somebody, and Recent never shows one account's entries
// to another. These tests hold that.

import (
	"strings"
	"testing"
)

func TestAnEntryWithAnAccountIsInvisibleToEveryoneElse(t *testing.T) {
	reset(t)
	add(&Entry{Service: "mail", Text: "lawyer@example.com — Re: the settlement", Account: "alice"})
	add(&Entry{Service: "news", Text: "Something happened"})

	for _, viewer := range []string{"", "bob"} {
		for _, e := range Recent(100, viewer) {
			if strings.Contains(e.Text, "settlement") {
				t.Errorf("viewer %q can see alice's mail: %q", viewer, e.Text)
			}
		}
	}

	var sawOwn bool
	for _, e := range Recent(100, "alice") {
		if strings.Contains(e.Text, "settlement") {
			sawOwn = true
		}
	}
	if !sawOwn {
		t.Error("alice cannot see her own entry")
	}
}

// The public half of the same property: an entry with no account is meant to
// be read by somebody with no session, so nothing personal may be announced
// without one.
func TestAnEntryWithNoAccountIsVisibleToEveryone(t *testing.T) {
	reset(t)
	add(&Entry{Service: "news", Text: "Headline"})

	got := Recent(100, "")
	if len(got) != 1 || got[0].Text != "Headline" {
		t.Fatalf("a signed-out reader sees %v, want the one public entry", got)
	}
}

// Deleting an account takes their own entries with it. The public ones stay:
// a post that was published still was, and whether it survives is the blog
// service's decision, not this one's.
func TestDeletingAnAccountLeavesThePublicEntries(t *testing.T) {
	reset(t)
	add(&Entry{Service: "mail", Text: "theirs", Account: "alice"})
	add(&Entry{Service: "news", Text: "everyone's"})

	DeleteByAccount("alice")

	got := Recent(100, "alice")
	if len(got) != 1 || got[0].Text != "everyone's" {
		t.Fatalf("after deletion the timeline holds %v, want the public entry alone", got)
	}
}

// Fixing the call sites stops new leaks; it does not unpublish the old ones,
// which sit in stream.json and go on being served. They cannot survive the
// move, because a console event had a type and a content where an entry has a
// service and a text — so they arrive empty in the fields that matter, and
// valid drops them.
func TestConsoleEntriesDoNotSurviveTheMove(t *testing.T) {
	// What json.Unmarshal makes of {"type":"reminder","content":"⏰ PRIVATE
	// Dentist about the biopsy results","author_id":"someone"}: no service, no
	// text, no time.
	for _, e := range []*Entry{
		{},
		{Service: "reminder"},
		{Text: "⏰ PRIVATE Dentist about the biopsy results"},
	} {
		if valid(e) {
			t.Errorf("a console event survived as %+v", e)
		}
	}
}

// Text is escaped, never rendered. The console ran everything through the
// markdown renderer because the agent answered in markdown into it; nothing
// writes prose here now, and a service's one-line fact has no reason to carry
// markup.
func TestEntryTextIsEscaped(t *testing.T) {
	out := renderEntry(&Entry{Service: "news", Text: `<img src=x onerror=alert(1)>`})
	if strings.Contains(out, "<img src=x") {
		t.Errorf("rendered output carries live markup:\n%s", out)
	}
	if !strings.Contains(out, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Errorf("text was not escaped:\n%s", out)
	}
}

// The URL is an attribute, and an entry announcing one is announcing a link
// somebody else wrote — a feed item, a fetched page.
func TestEntryURLCannotBreakOutOfItsAttribute(t *testing.T) {
	out := renderEntry(&Entry{Service: "news", Text: "story", URL: `" onmouseover="alert(1)`})
	if strings.Contains(out, `onmouseover="alert`) {
		t.Errorf("url escaped its attribute:\n%s", out)
	}
}
