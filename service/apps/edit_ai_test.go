package apps

import (
	"strings"
	"testing"
	"time"

	"mu/service/apps/micro"
)

func withApp(t *testing.T, a *App) func() {
	t.Helper()
	mutex.Lock()
	apps[a.Slug] = a
	mutex.Unlock()
	return func() {
		mutex.Lock()
		delete(apps, a.Slug)
		mutex.Unlock()
	}
}

func sampleApp() *App {
	return &App{
		ID:       "id_1",
		Slug:     "expenses",
		Name:     "Expenses",
		AuthorID: "alice",
		HTML:     "<html>original</html>",
		Spec: &micro.Spec{
			Type:   "tracker",
			Title:  "Expenses",
			Emoji:  "💸",
			Fields: []micro.Field{{Name: "Item", Type: "text"}, {Name: "Amount", Type: "number"}},
			Sum:    "Amount",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// Only the author may edit. Checked inside EditMicroApp so every caller — web
// form, API, agent tool — is covered by the same rule.
func TestEditMicroAppRejectsNonAuthor(t *testing.T) {
	defer withApp(t, sampleApp())()

	_, err := EditMicroApp("mallory", "expenses", "add a Notes field")
	if err == nil {
		t.Fatal("expected an error for a non-author")
	}
	if !strings.Contains(err.Error(), "only the author") {
		t.Errorf("error = %q, want an ownership failure", err)
	}
}

func TestEditMicroAppRequiresInstruction(t *testing.T) {
	defer withApp(t, sampleApp())()

	if _, err := EditMicroApp("alice", "expenses", "   "); err == nil {
		t.Error("expected an error for an empty instruction")
	}
}

func TestEditMicroAppUnknownSlug(t *testing.T) {
	if _, err := EditMicroApp("alice", "nope", "change it"); err == nil {
		t.Error("expected an error for a missing app")
	}
}

// Apps built before specs were stored cannot be edited structurally. The
// message has to say so rather than silently regenerating and losing work.
func TestEditMicroAppWithoutSpecExplainsWhy(t *testing.T) {
	a := sampleApp()
	a.Spec = nil
	defer withApp(t, a)()

	_, err := EditMicroApp("alice", "expenses", "add a Notes field")
	if err == nil {
		t.Fatal("expected an error when there is no spec")
	}
	if !strings.Contains(err.Error(), "fork") {
		t.Errorf("error = %q, should tell the user what to do instead", err)
	}
}

// A rollback has to restore the spec with the markup, or a later edit would
// work from a spec describing an app the HTML no longer matches.
func TestSnapshotVersionCapturesSpec(t *testing.T) {
	a := sampleApp()
	snapshotVersion(a, "Initial version")

	if len(a.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(a.Versions))
	}
	v := a.Versions[0]
	if v.Spec == nil {
		t.Fatal("version did not capture the spec")
	}
	if v.Spec.Title != "Expenses" || v.HTML != a.HTML {
		t.Errorf("version snapshot does not match the app: %+v", v)
	}
}

func TestSummariseTrimsLongInstructions(t *testing.T) {
	long := strings.Repeat("make it better ", 20)
	got := summarise(long)
	if len(got) > 60 {
		t.Errorf("summary is %d chars, want <= 60", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a truncated summary should be marked: %q", got)
	}
	if got := summarise("  add a  Notes field "); got != "add a Notes field" {
		t.Errorf("summary = %q, want whitespace collapsed", got)
	}
}

// micro.Edit must refuse the cases that would otherwise reach the model.
func TestMicroEditGuards(t *testing.T) {
	if _, err := micro.Edit(nil, "change it"); err == nil {
		t.Error("expected an error with no spec")
	}
	if _, err := micro.Edit(&micro.Spec{Type: "counter", Title: "x"}, ""); err == nil {
		t.Error("expected an error with no instruction")
	}
}
