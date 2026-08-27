package user

// The way to write a post, from the page your posts are on.

import (
	"strings"
	"testing"
)

// Your own profile offers it. Somebody else's does not.
//
// The page lists what a person has written, so on your own it is the obvious
// place to add to the list and was the one place you could not. On a
// stranger's page the same control would be posting as them.
func TestYouCanPostFromYourOwnProfile(t *testing.T) {
	GetUserPosts = func(id, name string) []UserPost { return nil }
	t.Cleanup(func() { GetUserPosts = nil })

	mine := postLink("Asim", true)
	if !strings.Contains(mine, `href="/blog?write=true`) {
		t.Errorf("the control does not open the compose form:\n%s", mine)
	}
	// Composing stays in the one place that has the whole form. A second one
	// on the profile was two text boxes of the same shape doing different
	// jobs — a 140-character status above a post with a fifty-character floor.
	if strings.Contains(mine, "<form") || strings.Contains(mine, "<textarea") {
		t.Errorf("the profile carries a second compose form:\n%s", mine)
	}
	// It comes back here rather than dropping you on the blog index.
	if !strings.Contains(mine, "return=%2F%40asim") {
		t.Errorf("posting from your profile does not return to it:\n%s", mine)
	}

	if theirs := postLink("Asim", false); theirs != "" {
		t.Errorf("somebody else's profile offered a way to post as them:\n%s", theirs)
	}
}

// It is a button, in the vocabulary this page actually has.
//
// The one before it wore "lcta lcta-second", which is defined in the landing
// shell's own <style> block and in nothing mu.css serves — so on a page that
// renders through the app shell it had no background, no padding and no
// border. Confirmed in a browser: background rgba(0,0,0,0), padding 0px.
//
// A class name that resolves on some pages and not others is invisible to
// everything but looking, which is why this pins the class rather than
// trusting the next reader to check.
func TestTheProfileActionsAreButtonsTheAppCanStyle(t *testing.T) {
	GetUserPosts = func(id, name string) []UserPost { return nil }
	AddressFor = func(id string) string { return id + "@example.test" }
	t.Cleanup(func() { GetUserPosts = nil; AddressFor = nil })

	for what, got := range map[string]string{
		"New post":     postLink("asim", true),
		"Send message": writeLink("henrik"),
	} {
		if !strings.Contains(got, `class="pill"`) {
			t.Errorf("%s is not styled as a button this app defines:\n%s", what, got)
		}
		if strings.Contains(got, "lcta") {
			t.Errorf("%s uses the landing page's class, which mu.css does not "+
				"define — it renders as plain text:\n%s", what, got)
		}
	}
}

// No blog behind it, no button.
//
// internal/user cannot import service/blog — the layering rule, and why posts
// arrive through GetUserPosts at all. That hook being nil is the honest test
// for whether /blog is there: a button aimed at a route nothing serves is a
// button that 404s.
func TestNoButtonWithNoBlogToPostTo(t *testing.T) {
	GetUserPosts = nil
	if got := postLink("asim", true); got != "" {
		t.Errorf("a New post button was drawn with nowhere to post:\n%s", got)
	}
}
