package user

// Writing a post from the page your posts are on.
//
// The profile lists what somebody has written and, on your own, gave you no
// way to write another — the box for that lives at /blog/write, two clicks and
// a different screen away, and lands you back on /blog when you are done. So
// the page that is about your posts was the one place you could not make one.
//
// This is the same form, smaller, where the list is. Subject and body, because
// those are the two fields a post has that a status does not; tags and
// visibility are left to the full form at /blog?write=true, still there for a
// post worth dressing.
//
// # It posts to the blog, not to a second store
//
// /blog is the handler, with every rule it enforces — the length floor, the
// spam checks, the moderation event. A shortcut that skipped them would be a
// second way in with a different set of rules, which is how the rules stop
// being rules. The only thing the shortcut adds is a return field, so the form
// sends you back to the page you wrote from.
//
// # Why the hook decides whether it renders
//
// internal/user cannot import service/blog — the layering rule, and the reason
// posts arrive here through GetUserPosts in the first place. That hook is nil
// on an instance built without the blog, which makes it the honest test for
// whether /blog is there to post to: a form pointed at a route nothing serves
// is a button that 404s.

import (
	"html"
	"strconv"
	"strings"
)

// postMinimum is the blog's own floor, repeated here so the box can say it
// before the server refuses. See handlePost in service/blog.
const postMinimum = 50

// postBox is the compose form on your own profile, or nothing on somebody
// else's — and nothing on an instance with no blog behind it.
//
// No CSRF field: /blog does not check one, and a token the receiver ignores
// reads as protection that is not there. If posting gains that check, the
// field belongs on every form that posts, this one included.
func postBox(accountID string, own bool) string {
	if !own || GetUserPosts == nil {
		return ""
	}
	back := "/@" + strings.ToLower(accountID)
	return `<form class="pf-post-form" method="post" action="/blog">` +
		`<input type="hidden" name="return" value="` + html.EscapeString(back) + `">` +
		`<input class="pf-post-title" type="text" name="title" placeholder="Subject (optional)">` +
		`<textarea class="pf-post-body" name="content" rows="3" required ` +
		`placeholder="Write a post"></textarea>` +
		`<div class="pf-post-foot">` +
		`<span class="pf-post-hint">At least ` + strconv.Itoa(postMinimum) +
		` characters. <a href="/blog?write=true">Tags and visibility</a></span>` +
		`<button type="submit" class="pill">Post</button>` +
		`</div></form>`
}
