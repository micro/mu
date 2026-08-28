package apps

import (
	"os"
	"strings"
	"testing"
)

// An app written as a document can be changed.
//
// It could not. EditMicroApp requires a Spec, and an app from writeApp — the
// builder that writes real HTML instead of filling in one of three shapes — has
// none. So the AI edit panel answered every one of them with "this app was
// built before edits were supported, fork it to get an editable copy", which
// was a sentence about apps made before specs existed and became a sentence
// about the newest apps on the instance.
//
// A model is not called here. What is asserted is the dispatch: given a
// document app, the code takes the document path rather than the refusal.
func TestADocumentAppIsNotToldToForkItself(t *testing.T) {
	const who = "doc_author"
	mutex.Lock()
	apps["docapp"] = &App{Slug: "docapp", Name: "Doc", AuthorID: who,
		HTML: "<!doctype html><html><body>hi</body></html>"}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		delete(apps, "docapp")
		mutex.Unlock()
	}()

	_, err := EditHTMLApp(who, "docapp", "make it dark")
	// Reaching the model is not the point and will not happen in a test; what
	// must not happen is a refusal about specs before it gets that far.
	if err != nil && strings.Contains(err.Error(), "fork") {
		t.Errorf("a document app is still told to fork itself: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "spec") {
		t.Errorf("a document app was routed to the spec editor: %v", err)
	}
}

// Whose app it is, checked once, for every door.
//
// The editor form, /code, the API and an agent's tool call all end up here, and
// ownership that is enforced per door is ownership that one door forgets.
func TestOnlyTheAuthorCanChangeAnApp(t *testing.T) {
	mutex.Lock()
	apps["mine"] = &App{Slug: "mine", Name: "Mine", AuthorID: "owner", HTML: "<html></html>"}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		delete(apps, "mine")
		mutex.Unlock()
	}()

	if _, err := editable("somebody_else", "mine", "change it"); err == nil {
		t.Error("somebody who does not own an app was allowed to change it")
	}
	if _, err := editable("owner", "mine", "   "); err == nil {
		t.Error("an empty instruction was accepted, which spends credits to do nothing")
	}
	if _, err := editable("owner", "no_such_app", "change it"); err == nil {
		t.Error("editing an app that does not exist was allowed")
	}
	if _, err := editable("owner", "mine", "change it"); err != nil {
		t.Errorf("the author cannot change their own app: %v", err)
	}
}

// The instruction reaches the model twice, and the reason is worth keeping.
//
// A long prompt is weighted at its end, and the end of this one is the closing
// tags of a document about to be rewritten. The instruction goes first so the
// document can be read in light of it, and last so it is the final thing said.
func TestTheInstructionBracketsTheDocument(t *testing.T) {
	q := editQuestion("<html>the app</html>", "make it dark")
	if strings.Count(q, "make it dark") != 2 {
		t.Errorf("the instruction does not bracket the document:\n%s", q)
	}
	if !strings.Contains(q, "<html>the app</html>") {
		t.Errorf("the current document is not in the question:\n%s", q)
	}
	if strings.Index(q, "make it dark") > strings.Index(q, "<html>") {
		t.Error("the document comes before the instruction, so it is read with no idea what for")
	}
}

// One loop decides whether a program is fit to keep.
//
// Building an app and changing one run the same scanner and the same tests. Two
// copies of that decision is two places for it to drift, and the one that
// drifts is the one added later — which here is the edit.
func TestBuildingAndEditingShareTheChecks(t *testing.T) {
	for _, f := range []string{"build.go", "edit_html.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "refinement{") {
			t.Errorf("%s does not go through the shared ask-check-ask-again loop, "+
				"so a document it accepts is judged by its own rules", f)
		}
	}
	// And the checks themselves stay in one function.
	b, _ := os.ReadFile("edit_html.go")
	if strings.Contains(string(b), "ScanApp(") || strings.Contains(string(b), "TestHTML(") {
		t.Error("edit_html.go runs the checks itself instead of through buildProblems")
	}
}

// The turn reports what it cost in attempts.
//
// A turn that took three tries and a turn that took one are different events.
// A page that says "done" for both teaches somebody the checks are decoration,
// and the next time one genuinely fails they will not believe it.
func TestATurnSaysHowManyAttemptsItTook(t *testing.T) {
	b, err := os.ReadFile("edit_html.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Attempts") {
		t.Error("an edit does not report how many attempts it took")
	}
}
