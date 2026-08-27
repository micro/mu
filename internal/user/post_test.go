package user

// The compose box on a profile.

import (
	"strings"
	"testing"
)

// Your own profile is where you write the next one. Somebody else's is not.
//
// The page lists what a person has written, so on your own it is the obvious
// place to add to the list and was the one place you could not — /blog/write
// was two clicks away on another screen. On a stranger's page a box would be
// writing as them.
func TestYouCanPostFromYourOwnProfile(t *testing.T) {
	GetUserPosts = func(id, name string) []UserPost { return nil }
	t.Cleanup(func() { GetUserPosts = nil })

	mine := postBox("Asim", true)
	if !strings.Contains(mine, `action="/blog"`) {
		t.Errorf("the box does not post to the blog, so it is a second store:\n%s", mine)
	}
	if !strings.Contains(mine, `name="title"`) || !strings.Contains(mine, `name="content"`) {
		t.Errorf("subject and body are the two fields a post has:\n%s", mine)
	}
	// It says the floor before the server refuses at it. handlePost rejects
	// anything shorter with a bare 400, which on a form is a page that just
	// went wrong.
	if !strings.Contains(mine, "50") {
		t.Errorf("nothing on the box says how long a post has to be:\n%s", mine)
	}
	// And it comes back here rather than dropping you on /blog.
	if !strings.Contains(mine, `name="return" value="/@asim"`) {
		t.Errorf("writing from your profile does not return to it:\n%s", mine)
	}

	if theirs := postBox("Asim", false); theirs != "" {
		t.Errorf("somebody else's profile came with a box to post as them:\n%s", theirs)
	}
}

// No blog behind it, no box.
//
// internal/user cannot import service/blog — the layering rule, and why posts
// arrive through GetUserPosts at all. That hook being nil is the honest test
// for whether /blog is there: a form aimed at a route nothing serves is a
// button that 404s.
func TestNoBoxWithNoBlogToPostTo(t *testing.T) {
	GetUserPosts = nil
	if got := postBox("asim", true); got != "" {
		t.Errorf("a compose box was drawn with nowhere to post:\n%s", got)
	}
}
