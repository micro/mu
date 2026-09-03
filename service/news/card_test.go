package news

// What the home card shows, when there is more of it than fits.

import (
	"strings"
	"testing"
	"time"
)

// The same feed makes the same card twice.
//
// It did not. The card is one headline per category and at most ten of them,
// and the ten were taken by iterating a map and breaking — Go randomises map
// order, so an instance with more than ten categories dropped a different two
// on every rebuild. The sort by time ran afterwards, on a selection that had
// already lost the wrong ones, which is why it looked ordered and was not.
//
// Eight feeds ship with this, so nothing showed the bug by default. feeds.json
// is an operator's file and a ninth category is the ordinary thing to add.
func TestTheCardPicksTheFreshestCategoriesAndPicksThemTwice(t *testing.T) {
	now := time.Now()
	var posts []*Post
	// Twelve categories, each one hour older than the last, so which ten belong
	// on the card is not a matter of opinion.
	for i := 0; i < 12; i++ {
		posts = append(posts, &Post{
			ID:       string(rune('a' + i)),
			Title:    "headline " + string(rune('a'+i)),
			Category: "cat" + string(rune('a'+i)),
			PostedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}

	first := cardPosts(posts)
	if len(first) != cardHeadlines {
		t.Fatalf("the card has %d headlines, want %d", len(first), cardHeadlines)
	}
	// The freshest ten, which means the two oldest categories are the ones left
	// out — not two chosen by the runtime.
	for _, p := range first {
		if p.Category == "catk" || p.Category == "catl" {
			t.Errorf("%s is on the card and is one of the two oldest categories", p.Category)
		}
	}
	if first[0].Category != "cata" {
		t.Errorf("the card leads with %s, not the newest", first[0].Category)
	}

	// And again, from the same input. A map iteration would differ here within
	// a handful of runs.
	for i := 0; i < 20; i++ {
		again := cardPosts(posts)
		for j := range again {
			if again[j].Category != first[j].Category {
				t.Fatalf("run %d differs at %d: %s then %s", i, j,
					first[j].Category, again[j].Category)
			}
		}
	}
}

// Two feeds that published in the same second still order the same way twice.
func TestATieIsBrokenByName(t *testing.T) {
	at := time.Now()
	posts := []*Post{
		{ID: "1", Category: "world", PostedAt: at},
		{ID: "2", Category: "crypto", PostedAt: at},
		{ID: "3", Category: "islam", PostedAt: at},
	}
	want := []string{"crypto", "islam", "world"}
	for i := 0; i < 20; i++ {
		got := cardPosts(posts)
		for j := range want {
			if got[j].Category != want[j] {
				t.Fatalf("run %d: order %s, want %s", i,
					strings.Join(categories(got), " "), strings.Join(want, " "))
			}
		}
	}
}

func categories(posts []*Post) []string {
	var out []string
	for _, p := range posts {
		out = append(out, p.Category)
	}
	return out
}

// What the prompt carries, and what it does not.
//
// The short form: titles under their topics. HeadlinesText is the tool's
// answer and carries a description, a source and an id per item, because a
// model that asked for the news wants to read one — this is paid for on every
// question whether or not it was about the news, so it holds the part that
// answers "what is happening" and nothing else.
func TestNowIsTitlesAndNothingElse(t *testing.T) {
	mutex.Lock()
	was := feed
	feed = []*Post{{
		ID: "1", Title: "Something happened", Category: "World",
		Description: "A long description that has no business being in every prompt",
		URL:         "https://example.test/a", PostedAt: time.Now(),
	}}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); feed = was; mutex.Unlock() })

	got := Now()
	if !strings.Contains(got, "Something happened") || !strings.Contains(got, "World") {
		t.Errorf("the headline is not in the block: %q", got)
	}
	if strings.Contains(got, "no business") {
		t.Errorf("the description is in the block, which is paid for on every question: %q", got)
	}
	if strings.Contains(got, "https://") {
		t.Errorf("a url is in the block: %q", got)
	}
	// And it says where to get more, or the model has no reason to believe
	// there is any.
	if !strings.Contains(got, "news_read") {
		t.Errorf("the block does not say how to read one in full: %q", got)
	}
}

// An instance whose feeds have not run says nothing, rather than saying it has
// nothing. A sentence about having no news would ride along on every question.
func TestNoNewsIsSilence(t *testing.T) {
	mutex.Lock()
	was := feed
	feed = nil
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); feed = was; mutex.Unlock() })

	if got := Now(); got != "" {
		t.Errorf("an empty feed wrote into the prompt: %q", got)
	}
}
