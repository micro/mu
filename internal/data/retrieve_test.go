package data

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestQueryTerms(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"What is Muse AI personal agent?", []string{"muse", "ai", "personal", "agent"}},
		{"Tell me about UK news today", []string{"uk", "news"}},
		{"\"Meta\" OR muse* NEAR(foo)", []string{"meta", "muse", "near", "foo"}},
		{"what is météo London London", []string{"météo", "london"}},
		{"", nil},
	} {
		if got := QueryTerms(tc.q); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: %v", tc.q, got)
		}
	}
}

func TestRetrievePrivacyRankingAndBounds(t *testing.T) {
	resetSQLiteTestDB(t)
	for _, x := range []struct {
		id, kind, owner, title string
		public                 bool
	}{
		{"public", KindNews, "", "Muse personal agent", false},
		{"other", KindNews, "alice", "Muse personal agent", false},
		{"legacy", KindPost, "", "Muse personal agent", false},
		{"blog", KindPost, "", "Muse personal agent", true},
		{"video", KindVideo, "", "Muse personal agent", false},
	} {
		if err := IndexSQLite(x.id, x.kind, x.title, "Muse personal agent evidence", x.owner, map[string]any{"public": x.public, "url": "https://example.com/" + x.id}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Retrieve(context.Background(), []string{"muse"}, []string{KindNews, KindPost})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("privacy filter: %+v", got)
	}
	for _, p := range got {
		if p.ID != "public" && p.ID != "blog" {
			t.Fatal(p.ID)
		}
	}
	for i := 0; i < 10; i++ {
		if err := IndexSQLite(fmt.Sprint(i), KindNews, "Other headline", "Muse personal agent evidence", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	got, err = Retrieve(context.Background(), []string{"muse"}, []string{KindNews})
	if err != nil || len(got) != 5 || got[0].ID != "public" {
		t.Fatalf("ranking/cap: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Retrieve(ctx, []string{"muse"}, []string{KindNews}); err == nil {
		t.Fatal("ignored cancellation")
	}
	got, err = Retrieve(context.Background(), []string{`muse" OR secret`}, []string{KindNews})
	if err != nil || len(got) != 0 {
		t.Fatalf("query syntax escaped incorrectly: %+v %v", got, err)
	}
}

func TestRetrieveDateUsesPublicationNotReindexTime(t *testing.T) {
	resetSQLiteTestDB(t)
	now := time.Now().UTC()
	for _, x := range []struct {
		id string
		at time.Time
	}{{"old", now.Add(-48 * time.Hour)}, {"new", now.Add(-time.Hour)}} {
		if err := IndexSQLite(x.id, KindNews, "Muse", "Muse evidence", "", map[string]any{"posted_at": x.at}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Retrieve(context.Background(), []string{"muse"}, []string{KindNews}, now.Add(-24*time.Hour))
	if err != nil || len(got) != 1 || got[0].ID != "new" {
		t.Fatalf("date filter: %+v %v", got, err)
	}
}
