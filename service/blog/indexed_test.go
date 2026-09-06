package blog

// A post is dated by when it was written.
//
// Reported: a post from eight months ago rendering as "1 minute ago". Not a
// formatting bug — the archive reads posted_at out of an entry's metadata and
// falls back to when it indexed the row, and blog never wrote posted_at at all.
// Every post is re-indexed on boot, so the fallback fired for the whole blog on
// every restart and the archive was uniformly stamped with the last deploy.
//
// It was written out by hand in four places, which is why it stayed missing:
// each site was a plausible-looking map literal, and none of them was wrong on
// its own terms. This tests the one function they now share.

import (
	"fmt"
	"mu/internal/bookmarks"
	"sync"
	"testing"
	"time"

	"mu/internal/data"
)

func TestAPostIsDatedByWhenItWasWritten(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	written := time.Now().Add(-8 * 30 * 24 * time.Hour).Truncate(time.Second)
	indexPost(Post{
		ID:        "old-one",
		Title:     "Need help with go micro",
		Content:   "Asked a long time ago.",
		Author:    "someone",
		CreatedAt: written,
	})

	entry := data.ByID("old-one")
	if entry == nil {
		t.Fatal("the post was not indexed")
	}

	got := data.PostedAt(entry)
	if !got.Equal(written) {
		t.Errorf("the archive dates this post %v, want %v (when it was written)", got, written)
	}
	// And specifically not now, which is the bug: a fallback to IndexedAt is
	// within a second of this line running.
	if time.Since(got) < time.Minute {
		t.Errorf("an eight-month-old post is dated %v ago", time.Since(got).Round(time.Second))
	}

	// The rest of what a post is found and linked by, in the same test because
	// internal/data holds one database handle for the process and a second
	// t.TempDir() leaves it pointing at a directory that no longer exists.
	//
	// Checked at all because the four hand-written maps are now one: a field
	// dropped here is dropped from every post on the instance.
	indexPost(Post{
		ID: "linked", Title: "A title", Content: "Body",
		Author: "writer", Tags: "go,micro", CreatedAt: written,
	})
	linked := data.ByID("linked")
	if linked == nil {
		t.Fatal("the second post was not indexed")
	}
	for key, want := range map[string]string{
		"url":    "/blog/post?id=linked",
		"author": "writer",
		"tags":   "go,micro",
	} {
		if got, _ := linked.Metadata[key].(string); got != want {
			t.Errorf("metadata %q = %q, want %q", key, got, want)
		}
	}
	if public, ok := linked.Metadata["public"].(bool); !ok || !public {
		t.Fatal("public post lacks visibility declaration")
	}
	indexPost(Post{ID: "linked", Title: "Now private", Content: "private content", Private: true})
	if data.ByID("linked") != nil {
		t.Fatal("private post remains publicly indexed")
	}

	// Exercise real publication and editing, on both supported index backends.
	// The returned operation must already have reconciled archive visibility;
	// a delayed publication must never bring a now-private post back.
	originalBackend := data.UseSQLite
	oldPosts, oldMap, oldUnreadable := posts, postsMap, postsUnreadable
	t.Cleanup(func() {
		data.UseSQLite = originalBackend
		posts, postsMap, postsUnreadable = oldPosts, oldMap, oldUnreadable
		updateCache()
	})
	posts, postsMap, postsUnreadable = nil, map[string]*Post{}, false
	for _, sqlite := range []bool{true, false} {
		data.UseSQLite = sqlite
		t.Run(fmt.Sprintf("visibility/sqlite=%v", sqlite), func(t *testing.T) {
			if err := CreatePost("Public", "public body", "writer", "writer-id", "Tech", false); err != nil {
				t.Fatal(err)
			}
			id := posts[0].ID
			if _, err := bookmarks.Source(id); err != nil {
				t.Fatalf("new public post is not available before CreatePost returns: %v", err)
			}
			if err := UpdatePost(id, "Private", "private body", "Tech", true); err != nil {
				t.Fatal(err)
			}
			if _, err := bookmarks.Source(id); err == nil || data.ByID(id) != nil {
				t.Fatal("private post remains available to saved reading")
			}
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					if err := UpdatePost(id, fmt.Sprintf("Public %d", i), "public body", "Tech", false); err != nil {
						t.Error(err)
					}
				}(i)
			}
			wg.Wait()
			if err := UpdatePost(id, "Private again", "private body", "Tech", true); err != nil {
				t.Fatal(err)
			}
			if _, err := bookmarks.Source(id); err == nil || data.ByID(id) != nil {
				t.Fatal("concurrent publication restored a private post")
			}
			// A failed save restores the previous source and visibility.
			postsUnreadable = true
			if err := UpdatePost(id, "Failed public", "body", "Tech", false); err == nil {
				t.Fatal("unwritable source accepted a visibility change")
			}
			postsUnreadable = false
			if !postsMap[id].Private || data.ByID(id) != nil {
				t.Fatal("failed save changed private source or public index")
			}
			if err := UpdatePost(id, "Public before delete", "body", "Tech", false); err != nil {
				t.Fatal(err)
			}
			if err := DeletePost(id); err != nil {
				t.Fatal(err)
			}
			if _, err := bookmarks.Source(id); err == nil || data.ByID(id) != nil {
				t.Fatal("deleted post remains available to new reading references")
			}
			if err := CreatePost("Account post", "body", "writer", "deleted-writer", "Tech", false); err != nil {
				t.Fatal(err)
			}
			id = posts[0].ID
			DeletePostsByAuthor("deleted-writer")
			if _, err := bookmarks.Source(id); err == nil || data.ByID(id) != nil {
				t.Fatal("deleted account's post remains available to new reading references")
			}
		})
	}

}
