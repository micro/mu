package stream

// The console is public. /stream serves it with no session and stream_list
// answers an unauthenticated MCP caller, which is deliberate — it is the
// instance's own timeline, the sign that something is running here.
//
// That makes every write to it a publication. Three call sites in main.go
// forgot: a fired reminder posted its title, a standing instruction posted
// its title, and inbound mail posted the sender and the subject. A reminder
// somebody had written down privately — "Dentist about the biopsy results" —
// went to the open internet the moment it fired, along with the account id of
// whoever owned it.
//
// This test cannot reach into main.go, so it guards the property from this
// side: whatever is posted, a system event must not carry an account id in its
// metadata. The content half is guarded by the reviewer and by the comments at
// the call sites; the identity half is guarded here, because an event with no
// title but a named owner still tells a reader who is awake at 3am.

import (
	"strings"
	"testing"
)

func TestASystemEventCarriesNoAccountIdentity(t *testing.T) {
	resetStreamForTest(t)
	e := PostSystem("⏰ A reminder fired", map[string]any{"kind": TypeReminder})

	for key := range e.Metadata {
		switch strings.ToLower(key) {
		case "account", "account_id", "accountid", "owner", "user", "user_id", "from", "email":
			t.Errorf("a public system event carries %q=%v, which identifies whose it was",
				key, e.Metadata[key])
		}
	}
}

// Whatever else is true of the timeline, an event that reached it is readable
// by somebody with no account — so this asserts the shape a caller sees rather
// than trusting that every future call site remembers.
func TestEventsOnTheTimelineAreVisibleToEveryone(t *testing.T) {
	// System events are throttled per type, so a fresh timeline is needed for
	// the post below to land at all.
	resetStreamForTest(t)
	before := len(Recent(100, ""))
	PostSystem("📬 Mail arrived", map[string]any{"kind": TypeSystem})

	got := Recent(100, "")
	if len(got) <= before {
		t.Fatal("a posted system event did not reach the timeline")
	}
	// The point of the assertion: a signed-out reader sees it. Anything put
	// here is published, which is why the call sites must post no content that
	// belongs to one person.
	found := false
	for _, e := range got {
		if e.Content == "📬 Mail arrived" {
			found = true
			if e.Type != TypeSystem {
				t.Errorf("a system event has type %q", e.Type)
			}
		}
	}
	if !found {
		t.Error("the event is not visible to a viewer with no account")
	}
}

// Fixing the call sites stops new leaks; it does not unpublish the old ones,
// which sit in stream.json and go on being served to anybody with no session.
// They are removed on the way in.
func TestHistoricalLeaksArePurgedOnLoad(t *testing.T) {
	loaded := []*Event{
		// The three that leaked, each tagged with whose it was.
		{Type: TypeReminder, Content: "⏰ PRIVATE Dentist about the biopsy results",
			Metadata: map[string]any{"kind": TypeReminder, "account": "someone"}},
		{Type: TypeSystem, Content: "📬 Mail from lawyer@example.com — Re: the settlement",
			Metadata: map[string]any{"kind": TypeSystem, "account": "someone"}},
		{Type: TypeReminder, Content: "⏰ Therapy",
			Metadata: map[string]any{"owner": "someone"}},

		// Legitimate console entries, which name nobody and must survive.
		{Type: TypeSystem, Content: "⏰ A reminder fired", Metadata: map[string]any{"kind": TypeReminder}},
		{Type: TypeSystem, Content: "📬 Mail arrived", Metadata: map[string]any{"kind": TypeSystem}},
		{Type: TypeSystem, Content: "Headlines updated"},

		// A person's own post is theirs to publish and is not touched.
		{Type: TypeUser, AuthorID: "someone", Content: "hello everyone",
			Metadata: map[string]any{"account": "someone"}},
	}

	kept := purgePrivate(loaded)

	for _, e := range kept {
		for _, secret := range []string{"biopsy", "settlement", "Therapy"} {
			if strings.Contains(e.Content, secret) {
				t.Errorf("a leaked entry survived the purge: %q", e.Content)
			}
		}
	}
	if len(kept) != 4 {
		t.Fatalf("purge kept %d entries, want the 3 anonymous ones plus the user's own post", len(kept))
	}

	var sawUserPost bool
	for _, e := range kept {
		if e.Type == TypeUser && e.Content == "hello everyone" {
			sawUserPost = true
		}
	}
	if !sawUserPost {
		t.Error("somebody's own public post was purged; it was theirs to publish")
	}
}
