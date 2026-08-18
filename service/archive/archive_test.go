package archive

// One question across everything the instance has collected.

import (
	"context"
	"strings"
	"testing"
	"time"

	"mu/internal/data"
)

// collected puts entries in the index for the duration of a test.
//
// Indexing is queued and done by background workers, so this waits for the last
// one to land rather than assuming it has. That is the real pipeline — a test
// that reached past it would pass against code that never indexes anything.
func collected(t *testing.T) {
	t.Helper()
	data.StartIndexing()
	for _, e := range []struct{ id, kind, title, body string }{
		{"n1", "news", "Rates held at 4%", "The central bank left rates unchanged."},
		{"n2", "news", "A quiet week for equities", "Little moved."},
		{"v1", "video", "Explaining interest rates", "A twenty minute explainer."},
		{"b1", "blog", "What rates mean for you", "Written here."},
	} {
		data.Index(e.id, e.kind, e.title, e.body, nil)
	}
	settle(t, "n1", "n2", "v1", "b1")
}

// settle waits for queued entries to be indexed.
func settle(t *testing.T, ids ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for _, id := range ids {
		for data.ByID(id) == nil {
			if time.Now().After(deadline) {
				t.Fatalf("%s was never indexed", id)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// The question that could not be asked before: one that crosses a service.
//
// news_search passes WithType("news"), the social page searches its own, the
// chat pulls a handful for context — so an agent could ask what the news said
// about rates and could not ask what this instance knows about them.
func TestOneSearchCrossesEveryService(t *testing.T) {
	collected(t)

	var rsp SearchResponse
	if err := (Server{}).Search(context.Background(), &SearchRequest{Query: "rates"}, &rsp); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"news", "video", "blog"} {
		if !strings.Contains(rsp.Text, want) {
			t.Errorf("the answer has nothing of kind %q in it:\n%s", want, rsp.Text)
		}
	}
}

// And it can still be narrowed, for a caller that knows which service holds it.
func TestASearchCanBeNarrowedToOneKind(t *testing.T) {
	collected(t)

	var rsp SearchResponse
	if err := (Server{}).Search(context.Background(),
		&SearchRequest{Query: "rates", Kind: "video"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "Explaining interest rates") {
		t.Errorf("the video is missing:\n%s", rsp.Text)
	}
	if strings.Contains(rsp.Text, "central bank") {
		t.Errorf("narrowing to video returned news:\n%s", rsp.Text)
	}
}

// Nothing found says so, rather than returning an empty answer somebody has to
// interpret.
func TestNothingFoundSaysSo(t *testing.T) {
	collected(t)

	var rsp SearchResponse
	if err := (Server{}).Search(context.Background(),
		&SearchRequest{Query: "zarquon"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "Nothing in the archive") {
		t.Errorf("an empty result reads %q", rsp.Text)
	}
}

// An empty query is a mistake worth naming: it would otherwise match the whole
// archive and return the twenty most recent things at random.
func TestAnEmptyQueryIsRefused(t *testing.T) {
	collected(t)
	var rsp SearchResponse
	if err := (Server{}).Search(context.Background(), &SearchRequest{}, &rsp); err == nil {
		t.Error("an empty query was accepted")
	}
}

// With no kind, List answers what is in the archive at all — the question
// nothing in the product could answer before. The admin page said the file was
// 366MB, which is a size and not an answer.
func TestListWithoutAKindSaysWhatIsHere(t *testing.T) {
	collected(t)

	var rsp ListResponse
	if err := (Server{}).List(context.Background(), &ListRequest{}, &rsp); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"news: 2", "video: 1", "blog: 1", "4 entries"} {
		if !strings.Contains(rsp.Text, want) {
			t.Errorf("the summary is missing %q:\n%s", want, rsp.Text)
		}
	}
}

// With one, it is the most recent of that kind.
func TestListWithAKindIsThatKind(t *testing.T) {
	collected(t)

	var rsp ListResponse
	if err := (Server{}).List(context.Background(), &ListRequest{Kind: "news"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "Rates held") || !strings.Contains(rsp.Text, "quiet week") {
		t.Errorf("the news is not in it:\n%s", rsp.Text)
	}
	if strings.Contains(rsp.Text, "Explaining interest") {
		t.Errorf("listing news returned video:\n%s", rsp.Text)
	}
}

// Somebody's private record is never in the archive, whoever is asking.
//
// The index holds owned entries as well as public ones, and this is the door
// that searches across every type at once — the exact shape of a leak, if the
// owner filter were left to the caller.
func TestAPrivateEntryIsNeverReturned(t *testing.T) {
	collected(t)
	data.IndexOwned("p1", "docs", "My salary review", "Confidential.", "somebody", nil)
	settle(t, "p1")

	var rsp SearchResponse
	if err := (Server{}).Search(context.Background(),
		&SearchRequest{Query: "salary"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rsp.Text, "Confidential") || strings.Contains(rsp.Text, "salary review") {
		t.Errorf("a private entry was returned by the public archive:\n%s", rsp.Text)
	}

	// And it is not counted, either. How many private records exist is not
	// archive statistics.
	for _, k := range data.Kinds() {
		if k.Name == "docs" {
			t.Errorf("private entries are counted in the archive: %+v", k)
		}
	}
}

// The same, against the index that is actually live.
//
// The two backends are separate implementations of the owner filter — a map
// comparison in one, a SQL clause in the other — and this is the door that
// searches every type at once. A leak would be in whichever one was not
// exercised, so both are.
func TestAPrivateEntryIsNeverReturnedFromSQLiteEither(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	was := data.UseSQLite
	data.UseSQLite = true
	t.Cleanup(func() { data.UseSQLite = was })

	data.EnsureFTS()
	data.Index("sq-pub", "news", "Rates held", "The central bank left rates unchanged.", nil)
	data.IndexOwned("sq-priv", "docs", "My salary review", "Rates of pay, confidential.", "somebody", nil)
	settle(t, "sq-pub", "sq-priv")

	var rsp SearchResponse
	if err := (Server{}).Search(context.Background(), &SearchRequest{Query: "rates"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "Rates held") {
		t.Errorf("the public entry is missing, so this proves nothing:\n%s", rsp.Text)
	}
	if strings.Contains(rsp.Text, "confidential") || strings.Contains(rsp.Text, "salary") {
		t.Errorf("SQLite returned a private entry to the public archive:\n%s", rsp.Text)
	}
}
