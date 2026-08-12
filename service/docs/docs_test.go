package docs

// A document is a title and a body.
//
// The service was a record store wearing the word: docs_create took a collection
// name and a bag of JSON, so a person opening the page had to type JSON, and the
// handler's commonest error branch existed for when they got it wrong. These
// tests hold the shape it should have had — write, read, list, delete, over
// something a person would call a document.

import (
	"context"
	"os"
	"strings"
	"testing"

	"mu/internal/service"
	"mu/internal/userdb"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-docs-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func ctxFor(owner string) context.Context {
	return service.WithAccount(context.Background(), owner)
}

func TestWriteReadListDelete(t *testing.T) {
	const owner = "asim"

	var wrote WriteResponse
	err := Server{}.Write(ctxFor(owner), &WriteRequest{
		Title:   "Notes on the Q3 plan",
		Content: "# Q3\n\nThe short version is that we ship the inbox.",
	}, &wrote)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if wrote.Doc == nil || wrote.Doc.ID == "" {
		t.Fatalf("Write returned no document: %+v", wrote)
	}
	id := wrote.Doc.ID

	// Read by id, and by exact title — an agent that has just listed has the
	// id, and a person telling an agent what they mean has the title.
	for _, req := range []*ReadRequest{{ID: id}, {Title: "Notes on the Q3 plan"}} {
		var got ReadResponse
		if err := (Server{}).Read(ctxFor(owner), req, &got); err != nil {
			t.Fatalf("Read(%+v): %v", req, err)
		}
		if got.Doc == nil || got.Doc.ID != id {
			t.Fatalf("Read(%+v) = %+v, want the document just written", req, got.Doc)
		}
		if !strings.Contains(got.Text, "we ship the inbox") {
			t.Errorf("Read text is missing the body: %q", got.Text)
		}
	}

	var listed ListResponse
	if err := (Server{}).List(ctxFor(owner), &ListRequest{}, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Docs) != 1 {
		t.Fatalf("List returned %d documents, want 1", len(listed.Docs))
	}
	// Both halves: prose for a model, the same documents as data for a program.
	if !strings.Contains(listed.Text, "Notes on the Q3 plan") || !strings.Contains(listed.Text, id) {
		t.Errorf("List text should name the document and its id: %q", listed.Text)
	}
	// The list is a list, not every document in full — a caller choosing one
	// should not be handed all of them.
	if strings.Contains(listed.Text, "The short version is that we ship") {
		t.Error("List returned whole bodies rather than a snippet")
	}

	var del DeleteResponse
	if err := (Server{}).Delete(ctxFor(owner), &DeleteRequest{ID: id}, &del); err != nil {
		t.Fatal(err)
	}
	if Get(owner, id) != nil {
		t.Error("the document survived deletion")
	}
}

// TestWritingWithAnIDReplacesRatherThanDuplicates — editing a document twice
// should leave one document, and it should keep the id anybody already holds.
func TestWritingWithAnIDReplacesRatherThanDuplicates(t *testing.T) {
	const owner = "editor"

	var first WriteResponse
	if err := (Server{}).Write(ctxFor(owner), &WriteRequest{Title: "Draft", Content: "one"}, &first); err != nil {
		t.Fatal(err)
	}
	var second WriteResponse
	if err := (Server{}).Write(ctxFor(owner), &WriteRequest{
		ID: first.Doc.ID, Title: "Draft", Content: "two",
	}, &second); err != nil {
		t.Fatal(err)
	}
	if second.Doc.ID != first.Doc.ID {
		t.Errorf("editing changed the id from %s to %s", first.Doc.ID, second.Doc.ID)
	}
	if docs := All(owner, "", 0); len(docs) != 1 {
		t.Fatalf("editing left %d documents", len(docs))
	}
	if d := Get(owner, first.Doc.ID); d == nil || d.Content != "two" {
		t.Errorf("the edit did not stick: %+v", d)
	}
}

func TestADocumentNeedsATitle(t *testing.T) {
	var rsp WriteResponse
	if err := (Server{}).Write(ctxFor("someone"), &WriteRequest{Content: "body only"}, &rsp); err == nil {
		t.Error("wrote a document with no title")
	}
}

func TestDocumentsAreScopedToTheirOwner(t *testing.T) {
	var mine WriteResponse
	if err := (Server{}).Write(ctxFor("alice"), &WriteRequest{Title: "Mine", Content: "private"}, &mine); err != nil {
		t.Fatal(err)
	}
	if Get("bob", mine.Doc.ID) != nil {
		t.Error("bob can read alice's document by id")
	}
	var bobs ListResponse
	if err := (Server{}).List(ctxFor("bob"), &ListRequest{}, &bobs); err != nil {
		t.Fatal(err)
	}
	for _, d := range bobs.Docs {
		if d.ID == mine.Doc.ID {
			t.Error("alice's document appeared in bob's list")
		}
	}
}

func TestSearchMatchesTitleAndBody(t *testing.T) {
	const owner = "searcher"
	for _, d := range []struct{ title, body string }{
		{"Shipping plan", "the inbox comes first"},
		{"Unrelated", "nothing to see"},
	} {
		var rsp WriteResponse
		if err := (Server{}).Write(ctxFor(owner), &WriteRequest{Title: d.title, Content: d.body}, &rsp); err != nil {
			t.Fatal(err)
		}
	}
	if got := All(owner, "shipping", 0); len(got) != 1 || got[0].Title != "Shipping plan" {
		t.Errorf("searching titles found %d", len(got))
	}
	if got := All(owner, "inbox", 0); len(got) != 1 || got[0].Title != "Shipping plan" {
		t.Errorf("searching bodies found %d", len(got))
	}
}

func TestDocumentsRefuseAnAnonymousCaller(t *testing.T) {
	var rsp ListResponse
	if err := (Server{}).List(context.Background(), &ListRequest{}, &rsp); err == nil {
		t.Error("listed documents with nobody signed in")
	}
}

// TestAnAgentsDocumentsAreNotAnAppsRecords — apps persist in their own
// namespace, which is what keeps one app's storage out of another's and out of
// this. That was once documented the other way round, claiming an agent could
// "put something where an app will find it"; anybody who built on it would have
// found their records simply absent, with nothing erroring to say why.
func TestAnAgentsDocumentsAreNotAnAppsRecords(t *testing.T) {
	const owner = "asim"

	var rsp WriteResponse
	if err := (Server{}).Write(ctxFor(owner), &WriteRequest{Title: "From the agent", Content: "x"}, &rsp); err != nil {
		t.Fatal(err)
	}
	recs, err := userdb.List("apps/some-app", owner, collection, "mine", nil, "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Errorf("an app can see %d of the caller's documents", len(recs))
	}
}
