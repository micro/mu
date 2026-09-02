package home

// What is not on the front door.
//
// The page is a wordmark, a box, a row of doors and the day. Everything else
// that could sit under the box has to earn the space, and two things that could
// do not:
//
//   - The agent picker, signed in or out. Choosing which of your agents answers
//     is not a front-door question — arriving and asking one thing is the
//     motion here, and picking a specialist first is the opposite of it. Home
//     is the customised surface and has it.
//   - Read-aloud, for a stranger. A guest does get a real answer here, so this
//     is not the old "no answer can arrive" argument. It is that a checkbox
//     about how answers are delivered is furniture in front of somebody who has
//     not asked anything yet.

import (
	"os"
	"strings"
	"testing"
)

func TestTheFrontDoorHasNoAgentPicker(t *testing.T) {
	for _, who := range []string{"", "somebody"} {
		if strings.Contains(indexBody(who), `id="mu-chat-agent"`) {
			state := "signed out"
			if who != "" {
				state = "signed in"
			}
			t.Errorf("%s, the front door offers an agent picker. It belongs on Home,\n"+
				"which is the customised surface; here it asks somebody to choose a\n"+
				"specialist before they have asked anything.", state)
		}
	}
}

func TestAStrangerIsNotOfferedReadAloud(t *testing.T) {
	if strings.Contains(indexBody(""), `id="mu-chat-say"`) {
		t.Error("the first screen a stranger sees carries a checkbox about how\n" +
			"answers are read out, before they have asked anything")
	}
	if !strings.Contains(indexBody("somebody"), `id="mu-chat-say"`) {
		t.Error("somebody signed in has no read-aloud on the front door — they have\n" +
			"asked for an account, and this is the surface they land on")
	}
}

// And Home keeps both: it is where a person configures how they work.
func TestHomeKeepsThePicker(t *testing.T) {
	b, err := os.ReadFile("home.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, "OfferAgentPicker: viewerID != \"\"") {
		t.Error("Home no longer offers the agent picker, so an agent somebody made\n" +
			"can be reached from nowhere they would naturally talk to it")
	}
}
