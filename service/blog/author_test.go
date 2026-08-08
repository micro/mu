package blog

// A profile shows the posts the blog says are yours.
//
// The blog links an author at /@<id> and displays their name; the profile
// looked posts up by name. So the link and the lookup only agreed when an
// account's display name happened to equal the string stored on the post. It
// did not for the system user — posts signed "Mu", id "micro" — so every
// digest on the front page linked to a profile with nothing on it. Renaming
// yourself did the same thing to your own back catalogue.

import "testing"

func TestPostsAreFoundByTheIdTheyAreLinkedBy(t *testing.T) {
	mutex.Lock()
	saved := posts
	posts = []*Post{
		{ID: "1", Title: "Digest", Author: "Mu", AuthorID: "micro"},
		{ID: "2", Title: "Mine", Author: "Asim", AuthorID: "asim"},
		// Written before AuthorID existed: no id, only a name.
		{ID: "3", Title: "Old", Author: "Mu"},
	}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); posts = saved; mutex.Unlock() })

	// The system user's display name is not its id, which is the case that
	// was broken.
	got := GetPostsByAuthorID("micro", "Mu")
	if len(got) != 2 {
		t.Fatalf("the system user has %d posts, want 2 (one by id, one legacy by name)", len(got))
	}

	// Someone else's posts do not leak in.
	for _, p := range got {
		if p.AuthorID == "asim" {
			t.Error("another account's post appeared on this profile")
		}
	}

	// And an account whose name never matched anything still gets its own.
	if got := GetPostsByAuthorID("asim", "Asim Aslam"); len(got) != 1 || got[0].ID != "2" {
		t.Errorf("a renamed account lost its posts: %v", got)
	}
}

// A legacy post with no id must not fall through to every profile that asks.
func TestALegacyPostDoesNotMatchEveryone(t *testing.T) {
	mutex.Lock()
	saved := posts
	posts = []*Post{{ID: "1", Author: "Mu"}}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); posts = saved; mutex.Unlock() })

	if got := GetPostsByAuthorID("someone", "Someone Else"); len(got) != 0 {
		t.Errorf("a post with no author id showed on an unrelated profile: %v", got)
	}
	if got := GetPostsByAuthorID("nameless", ""); len(got) != 0 {
		t.Errorf("an account with no display name matched a nameless post: %v", got)
	}
}
