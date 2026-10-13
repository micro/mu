package blog

import (
	"mu/internal/app"
	"mu/internal/auth"
)

// Only the system account's public digest and topic posts belong in the main
// blog automatically. A matching tag or display name alone grants no placement.
func generatedPost(p *Post) bool {
	return !p.Private && p.AuthorID == auth.MicroID && (hasTag(p.Tags, "digest") || hasTag(p.Tags, "opinion"))
}

// Move generated posts created under the former Community default. Explicitly
// archived posts have Community=false and are left alone, including on restart.
// Called during Load before the cached lists are built.
func migrateGeneratedPosts() {
	var changed []*Post
	for _, p := range posts {
		if p.Community && !p.Editorial && generatedPost(p) {
			p.Editorial, p.Community = true, false
			changed = append(changed, p)
		}
	}
	if len(changed) == 0 {
		return
	}
	if err := save(); err != nil {
		for _, p := range changed {
			p.Editorial, p.Community = false, true
		}
		app.Log("blog", "move generated posts to editorial: %v", err)
	}
}
