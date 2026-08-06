package blog

import "testing"

// An agent asked to "read the post about the migration" should not have to list
// everything, scan for a title, copy a uuid and call again.
func TestAPostCanBeFoundByWhatItIsCalled(t *testing.T) {
	mutex.Lock()
	prev := posts
	posts = []*Post{
		{ID: "a1", Title: "The migration, in full"},
		{ID: "b2", Title: "Weeknotes"},
		{ID: "c3", Title: "Weeknotes, again"},
	}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); posts = prev; mutex.Unlock() })

	if got := ByTitle("The migration, in full"); got == nil || got.ID != "a1" {
		t.Errorf("exact title did not resolve: %+v", got)
	}
	if got := ByTitle("the MIGRATION"); got == nil || got.ID != "a1" {
		t.Errorf("a prefix, case-insensitively, did not resolve: %+v", got)
	}
	if got := ByTitle("migration"); got == nil || got.ID != "a1" {
		t.Errorf("a unique substring did not resolve: %+v", got)
	}

	// Two plausible posts must not become a guess. Deleting the wrong one is
	// the failure this exists to avoid.
	if got := ByTitle("Weeknotes"); got == nil || got.ID != "b2" {
		t.Errorf("an exact match must win over a longer one that contains it: %+v", got)
	}
	if got := ByTitle("Week"); got != nil {
		t.Errorf("an ambiguous prefix resolved to %+v instead of refusing", got)
	}
	if got := ByTitle("nothing like this"); got != nil {
		t.Errorf("a miss returned %+v", got)
	}
	if got := ByTitle("   "); got != nil {
		t.Error("blank input resolved to a post")
	}
}

// Resolve prefers the id, and falls back to the name.
func TestResolvePrefersTheIdAndFallsBackToTheTitle(t *testing.T) {
	mutex.Lock()
	prev, prevMap := posts, postsMap
	posts = []*Post{{ID: "a1", Title: "The migration, in full"}}
	postsMap = map[string]*Post{"a1": posts[0]}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); posts, postsMap = prev, prevMap; mutex.Unlock() })

	if got := Resolve("a1", ""); got == nil || got.ID != "a1" {
		t.Errorf("id did not resolve: %+v", got)
	}
	if got := Resolve("", "migration"); got == nil || got.ID != "a1" {
		t.Errorf("title did not resolve when no id was given: %+v", got)
	}
	if got := Resolve("nosuchid", "migration"); got == nil || got.ID != "a1" {
		t.Errorf("an unknown id should fall through to the title: %+v", got)
	}
}
