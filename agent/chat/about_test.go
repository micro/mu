package chat

// The agent has to know it is the agent.
//
// From a real transcript. Somebody typed "@micro ?" in a private room and got
// back:
//
//	@micro is an agent on this instance.
//	Name: micro · Address: micro@micro.mu · Agent: yes
//	Currently online: no
//	Profile page: /@micro
//
// It had looked its own handle up in the directory. about() returned "" for
// any room with no title or summary — which is every conversation between two
// people — so the agent was handed the bare string "@micro ?" with no framing
// at all. With nothing saying it was @micro, that string is a username and a
// question mark, and looking a username up is a reasonable thing to do with
// one. So it reported its own profile, in its own voice, in the room it was
// speaking in, and said it was offline while saying it.
//
// The framing is the difference between being addressed and being mentioned,
// and only the room knows which.

import (
	"strings"
	"testing"
)

func TestTheAgentIsToldWhoItIs(t *testing.T) {
	// The case from the report: a private room, so no title, no summary, no
	// URL. This used to be the empty string.
	bare := about(spoken{Room: "priv_abc", Account: "asim", Text: "@micro ?"})
	if strings.TrimSpace(bare) == "" {
		t.Fatal("a room with no subject gives the agent no framing at all — which " +
			"is how \"@micro ?\" became a directory lookup on its own handle")
	}

	for _, want := range []string{
		"@micro",            // who it is
		"chat room",         // where it is
		"addressing you",    // what a mention of its name means
		"your own profile",  // and what not to answer with
		"your own presence", // it is speaking, so it is here
	} {
		if !strings.Contains(bare, want) {
			t.Errorf("the framing does not mention %q:\n%s", want, bare)
		}
	}
}

// And a room with a subject still says what the subject is.
//
// The framing was added in front of it, not instead of it. An item room is a
// discussion about a specific thing and the agent answering without knowing
// which thing is the older bug.
func TestARoomWithASubjectStillSaysWhatItIs(t *testing.T) {
	got := about(spoken{
		Room:    "news_123",
		Title:   "Rates held at 4%",
		Summary: "The committee voted 7-2 to hold.",
		URL:     "https://example.com/rates",
	})

	for _, want := range []string{
		"@micro",                    // still framed
		"Rates held at 4%",          // the subject
		"The committee voted 7-2",   // and what is known about it
		"https://example.com/rates", // and where it came from
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the framing lost %q:\n%s", want, got)
		}
	}

	// The subject comes after the framing: the agent needs to know what it is
	// before it is told what it is looking at.
	if strings.Index(got, "@micro") > strings.Index(got, "Rates held") {
		t.Errorf("the subject is in front of who the agent is:\n%s", got)
	}
}
