package blog

import (
	"mu/internal/auth"
	"testing"
)

func TestGeneratedPostPlacement(t *testing.T) {
	for _, tc := range []struct {
		name, owner, tags string
		private, want     bool
	}{
		{"world", auth.MicroID, "digest", false, true},
		{"topic", auth.MicroID, "opinion,Tech", false, true},
		{"private", auth.MicroID, "digest", true, false},
		{"community", "reader", "opinion,Tech", false, false},
		{"unrelated", auth.MicroID, "announcement", false, false},
		{"partial tag", auth.MicroID, "not-digest", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := generatedPost(&Post{AuthorID: tc.owner, Tags: tc.tags, Private: tc.private}); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestGeneratedPostMigrationPreservesArchiveAndPrivacy(t *testing.T) {
	old, unreadable := posts, postsUnreadable
	t.Cleanup(func() { posts, postsUnreadable = old, unreadable })
	postsUnreadable = false
	digest := &Post{AuthorID: auth.MicroID, Tags: "digest", Community: true}
	topic := &Post{AuthorID: auth.MicroID, Tags: "opinion,Tech", Community: true}
	archived := &Post{AuthorID: auth.MicroID, Tags: "digest"}
	private := &Post{AuthorID: auth.MicroID, Tags: "opinion,Tech", Community: true, Private: true}
	human := &Post{AuthorID: "reader", Tags: "digest", Community: true}
	posts = []*Post{digest, topic, archived, private, human}
	migrateGeneratedPosts()
	migrateGeneratedPosts()
	for _, p := range []*Post{digest, topic} {
		if !p.Editorial || p.Community {
			t.Fatal("generated post was not moved")
		}
	}
	for _, p := range []*Post{archived, private, human} {
		if p.Editorial {
			t.Fatal("moved archived, private or human post")
		}
	}
	if !private.Community || !human.Community || archived.Community {
		t.Fatal("changed original placement")
	}
	// A failed store write must not leave an unpersisted placement in memory.
	digest.Editorial, digest.Community = false, true
	postsUnreadable = true
	migrateGeneratedPosts()
	if digest.Editorial || !digest.Community {
		t.Fatal("failed migration was not rolled back")
	}
}
