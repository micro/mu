package user

// Writing a post from the page your posts are on.
//
// The profile lists what somebody has written and, on your own, gave you no
// way to write another — the box for that is behind /blog?write=true, and it
// dropped you on /blog when you were done. So the page that is about your
// posts was the one place you could not make one.
//
// # Why this is a button and not a box
//
// It was the box: subject, body, Post, sitting in the head of the page. It
// read wrong and the reason is what the head of the page is. That block is
// identity — the name, the tick, when they joined, what they are up to — and a
// compose form is not identity, so it arrived as a second thing stapled on.
//
// Worse, it put two text inputs one above the other that look identical and
// are not: a 140-character status and a post with a fifty-character floor. Two
// boxes of the same shape doing different jobs is a worse failure than a
// missing shortcut, because the missing shortcut is at least honest about
// where composing happens.
//
// So the shortcut is a link, in the same slot where somebody else's profile
// offers Send message. One action, where an action belongs, and composing
// stays in the one place that has the whole form — tags, visibility, the
// counter. What it carries is the return: post from here and you come back
// here, which was the only part of the box worth keeping.
//
// # Why the hook decides whether it renders
//
// internal/user cannot import service/blog — the layering rule, and the reason
// posts arrive here through GetUserPosts in the first place. That hook is nil
// on an instance built without the blog, which makes it the honest test for
// whether /blog is there to post to: a button pointed at a route nothing
// serves is a button that 404s.

import (
	"html"
	"net/url"
	"strings"
)

// postLink is the way to write one, on your own profile and nowhere else.
//
// Styled .pill, which is what an action looks like in this app — the Save
// beside the status is one. It is not lcta: that pair is defined in the
// landing pages' own <style> block and in nothing the app shell serves, so a
// button wearing it on this page has no background, no padding and no border.
// See writeLink, which had exactly that problem.
func postLink(accountID string, own bool) string {
	if !own || GetUserPosts == nil {
		return ""
	}
	back := "/@" + strings.ToLower(accountID)
	return `<p class="pf-write"><a class="pill" href="/blog?write=true&amp;return=` +
		html.EscapeString(url.QueryEscape(back)) + `">New post</a></p>`
}
