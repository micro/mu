package agent

// The agents this instance ships with, listed where somebody can find them.
//
// # Signed out only, because signed in the roster already draws them
//
// This started as a second listing under your own agents, on the strength of a
// comment on the roster page saying the instance's agents were not listed
// there. They were: the loop over PlatformNames has drawn a row per built-in
// for a long time, and the comment underneath it was stale prose describing a
// state the code had since left. So Micro and Code appeared twice on one page.
//
// The lesson is not about agents. A comment that disagrees with the code five
// lines above it is worse than no comment, because it is read as a fact about
// the present and acted on — which is exactly what happened.
//
// What is left here is the page a stranger gets. They have no roster, so
// bouncing them to /login sends them to something empty by definition; "here is
// what this instance can already do, and the address of each" is a real answer,
// and it is the same argument /contact makes. See client.All, which exists
// because a way in that nothing enumerates quietly stops being maintained.

import (
	"html"
	"strings"

	"mu/agent/micro"
	"mu/internal/app"
)

// Builtin is one of the instance's own agents, as a page needs it.
type Builtin struct {
	ID          string
	Name        string
	Description string
	// Address is agent+<name>@<domain>, or "" on an instance with no mail
	// domain — where there is nothing to write to, the row does not claim
	// otherwise.
	Address string
	// Example is one thing worth asking it, in its own words. The first of the
	// agent's own Examples, which is also the placeholder on its page.
	Example string
}

// Builtins is every agent this instance ships, sorted, default first.
//
// Default first because it is the one to try: somebody reading this list is
// deciding whether to write to a specialist at all, and the general one is the
// answer for most of them.
func Builtins() []Builtin {
	var out []Builtin
	for _, id := range PlatformNames() {
		a := micro.Get(id)
		if a == nil {
			continue
		}
		b := Builtin{
			ID:          a.ID,
			Name:        strings.TrimSpace(a.Name),
			Description: strings.TrimSpace(a.Description),
			Address:     PlatformAddress(a.ID),
		}
		if b.Name == "" {
			b.Name = a.ID
		}
		if len(a.Examples) > 0 {
			b.Example = strings.TrimSpace(a.Examples[0])
		}
		out = append(out, b)
	}
	return out
}

// builtinsSection renders them, for the signed-out page.
//
// It took a `mine` flag once, to say "these come with the instance rather than
// being yours" on the signed-in roster — which turned out to be a second copy
// of a list that page had been drawing all along. One caller and one value is a
// decision wearing a choice's clothes, so the flag went with the duplicate.
//
// Empty on an instance with none registered, which is a build that has not
// loaded the platform agents rather than a state anybody is in.
func builtinsSection() string {
	list := Builtins()
	if len(list) == 0 {
		return ""
	}

	var b strings.Builder
	const head = "The agents on this instance"
	const note = "Each has its own address and its own tools. Write to one and it answers; " +
		"ask the first about anything and it decides which tools to reach for."

	b.WriteString(`<div class="card"><h3>` + head + `</h3>`)
	b.WriteString(`<p class="text-sm text-muted">` + note + `</p>`)
	b.WriteString(`<div class="bi-list">`)
	for _, a := range list {
		b.WriteString(`<div class="bi-row">`)
		b.WriteString(`<a class="bi-name" href="` + html.EscapeString(Path("", a.ID)) + `">` +
			html.EscapeString(a.Name) + `</a>`)
		if a.Description != "" {
			b.WriteString(`<span class="bi-what">` + html.EscapeString(a.Description) + `</span>`)
		}
		if a.Address != "" {
			b.WriteString(`<a class="bi-addr" href="mailto:` + html.EscapeString(a.Address) + `">` +
				html.EscapeString(a.Address) + `</a>`)
		}
		if a.Example != "" {
			b.WriteString(`<span class="bi-eg">&ldquo;` + html.EscapeString(a.Example) + `&rdquo;</span>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

// builtinsPage is the whole page for somebody with no account.
//
// A stranger has no roster, so the login form this used to be was a door with
// nothing behind it that they could see. What this instance can already do, and
// the address of each, is a better answer — and it is the same argument the
// contact card makes: the question "how do I use this" should not need an
// account to ask.
func builtinsPage() string {
	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(builtinsSection())
	b.WriteString(`<div class="card"><h3>Or make your own</h3>` +
		`<p class="text-sm text-muted">An agent you make gets its own name, its own ` +
		`instructions and its own address, and only the tools you give it. ` +
		`<a href="/signup">Make an account</a> and it is the first thing on this page.</p></div>`)
	b.WriteString(`</div>`)
	b.WriteString(builtinsCSS)
	return b.String()
}

// builtinsCSS is the row: a name, what it is for, where to write, and one thing
// to ask it. Four things of different weights, so they are a grid rather than a
// sentence — the same shape and the same reason as the contact card.
const builtinsCSS = `<style>
.bi-list{display:flex;flex-direction:column;margin:12px 0 0}
.bi-row{display:grid;grid-template-columns:minmax(90px,auto) minmax(0,1fr);
  gap:2px 14px;padding:10px 0;border-top:1px solid #f0f0f0}
.bi-row:first-child{border-top:0}
.bi-name{font-weight:700;font-size:14px;color:#111;text-decoration:none}
.bi-name:hover{text-decoration:underline}
.bi-what{font-size:14px;color:#333}
.bi-addr{grid-column:2;font-size:13px;color:#0645ad;text-decoration:none;word-break:break-all}
.bi-addr:hover{text-decoration:underline}
.bi-eg{grid-column:2;font-size:13px;color:#999;font-style:italic}
@media (max-width:520px){
  .bi-row{grid-template-columns:1fr}
  .bi-what,.bi-addr,.bi-eg{grid-column:1}
}
</style>`
