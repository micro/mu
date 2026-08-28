package apps

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// A turn changes the app you have, rather than making a second one.
//
// This is the whole difference between /code and the box it replaced. The box
// asked once: say "an expense tracker", get one; say "now make it dark" and get
// a second app called Dark. A conversation means the app is the subject and
// each thing said is a change to it.
//
// The mechanism is the slug travelling with the turn, which is why the assert
// is on the hidden field rather than on prose: no slug in the form is a new
// app on every submit, silently, and the page would look identical.
func TestTheBoxCarriesTheAppYouAreWorkingOn(t *testing.T) {
	a := &App{Slug: "tracker", Name: "Tracker", AuthorID: "coder"}

	editing := codeBox(a)
	if !strings.Contains(editing, `id="code-app" value="tracker"`) {
		t.Errorf("the box does not carry the app, so every turn starts a new one:\n%s", editing)
	}
	// And with nothing yet, it must not carry one.
	fresh := codeBox(nil)
	if !strings.Contains(fresh, `id="code-app" value=""`) {
		t.Errorf("the empty box names an app:\n%s", fresh)
	}

	// The button says which of the two this turn is. It said Build in both,
	// under a field asking what should change — a label that contradicts the
	// box above it is worse than none.
	if !strings.Contains(fresh, ">Build<") {
		t.Errorf("the first turn's button does not say it builds:\n%s", fresh)
	}
	if strings.Contains(editing, ">Build<") {
		t.Errorf("the button still says Build while changing an app that exists:\n%s", editing)
	}
}

// The transcript is the version list, read back.
//
// Nothing stores the conversation. If somebody adds a store for it later this
// test is what says why they should not: the two would disagree the first time
// a version was rolled back, and the log would still claim a change the app no
// longer has.
func TestTheTranscriptIsTheVersions(t *testing.T) {
	a := &App{
		Slug: "tracker", Name: "Tracker", Description: "an expense tracker",
		Versions: []Version{
			{Number: 1, Summary: "Initial version", SavedAt: time.Now()},
			{Number: 2, Summary: "make it dark", SavedAt: time.Now()},
			{Number: 3, Summary: "add a total row", SavedAt: time.Now()},
		},
	}
	got := codeTranscript(a)

	for _, want := range []string{"make it dark", "add a total row"} {
		if !strings.Contains(got, want) {
			t.Errorf("the transcript is missing a turn: %q\n%s", want, got)
		}
	}
	// The first turn is what was asked for, not the machine's word for it.
	if strings.Contains(got, "Initial version") {
		t.Errorf("the first turn reads as %q rather than what was asked:\n%s", "Initial version", got)
	}
	if !strings.Contains(got, "an expense tracker") {
		t.Errorf("the first turn does not say what was asked for:\n%s", got)
	}
	// Oldest first, so it reads down to the box.
	if strings.Index(got, "make it dark") > strings.Index(got, "add a total row") {
		t.Error("the transcript runs newest first, so the box is under the oldest turn")
	}
}

// An app with no history has no transcript, rather than an empty list with a
// border around it.
func TestANewAppHasNoTranscript(t *testing.T) {
	if got := codeTranscript(&App{Slug: "x"}); got != "" {
		t.Errorf("an app with no versions rendered a transcript: %q", got)
	}
}

// The app is on the page, running.
//
// Not a screenshot and not a description: the question each turn answers is
// "is that what I meant", and that is answered by looking at the thing.
func TestTheAppIsOnThePage(t *testing.T) {
	got := codeWorkspace(&App{Slug: "tracker", Name: "Tracker"})
	if !strings.Contains(got, `<iframe id="code-preview" src="/apps/tracker`) {
		t.Errorf("the app is not running on the page:\n%s", got)
	}
	// And the ways out of it: the app itself, its code, its history.
	for _, want := range []string{"/apps/tracker\"", "/apps/tracker/edit", "/apps/tracker/versions"} {
		if !strings.Contains(got, want) {
			t.Errorf("no link to %s:\n%s", want, got)
		}
	}
}

// A name is somebody's prose and goes through the escaper.
func TestAnAppNameIsEscapedOnTheCodePage(t *testing.T) {
	got := codeWorkspace(&App{Slug: "x", Name: `<script>alert(1)</script>`})
	if strings.Contains(got, "<script>alert") {
		t.Errorf("an app name is rendered as markup:\n%s", got)
	}
}

// /code needs a session, and says so rather than rendering an empty workspace.
func TestCodeIsNotOpenToStrangers(t *testing.T) {
	rec := httptest.NewRecorder()
	CodeHandler(rec, httptest.NewRequest("GET", "/code", nil))
	if rec.Code != 401 && rec.Code != 302 && rec.Code != 303 {
		t.Errorf("a signed-out visitor got %d from /code, want a refusal", rec.Code)
	}
}

// Describing an app happens in one place.
//
// There were two boxes that each claimed to build an app from a sentence, and
// only one of them could iterate. The other is gone; this fails if it comes
// back, because two front doors to the same act is how one of them quietly
// stops being the one that works.
func TestThereIsOneBoxThatBuildsAnAppFromASentence(t *testing.T) {
	b, err := os.ReadFile("apps.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if strings.Contains(src, `action="/apps/generate"`) {
		t.Error("the apps page still posts a description straight to /apps/generate. " +
			"Describing an app is /code, which checks the result and lets you say " +
			"what to change; a second box that asks once is the one that will rot.")
	}
	if !strings.Contains(src, `"/code"`) {
		t.Error("nothing on the apps page leads to /code, so the page that writes " +
			"apps is unreachable from the page that lists them")
	}
}

// The web button and the agent tool build with the same builder.
//
// They did not. BuildApp — the one that writes a document and runs the scanner
// and the tests over it until they pass — was wired to the agent tool alone,
// and the button a person clicked still went to the three-shape picker that
// build.go's own package comment describes as the thing it replaced. So "build
// me a unit converter" gave an agent a unit converter and gave a person a
// checklist called Unit Converter.
func TestEveryDoorBuildsWithTheSameBuilder(t *testing.T) {
	for _, f := range []string{"micro_build.go", "service.go", "code.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if !strings.Contains(src, "BuildApp(") {
			t.Errorf("%s builds an app without going through BuildApp", f)
		}
		// BuildMicroApp is the floor BuildApp falls back to, and calling it
		// directly is choosing the floor.
		if strings.Contains(src, "= BuildMicroApp(") {
			t.Errorf("%s calls BuildMicroApp directly, which picks one of three "+
				"shapes instead of writing the app", f)
		}
	}
}
