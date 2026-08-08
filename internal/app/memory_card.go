package app

// What the agent remembers about you, on the page about you.
//
// The agent writes memory from your conversations and main.go reads it back
// into the system prompt of every question you ask. Fifty entries, on disk, in
// every turn. A product that keeps notes on you and cannot show you the notes
// is asking for trust it has not earned, and the fix is a list with a delete
// button next to each row.
//
// This lived on /context, which has been removed. The rest of that page was a
// feature we were not doing well — a second home screen with a card picker on
// it — but the memory list was the one part of it that had to survive, because
// deleting it would put the notes back out of sight. /account is where it
// belongs anyway: it is the page about your account, and this is a thing your
// account holds.

import (
	htmlpkg "html"
	"net/http"
	"strings"

	"mu/internal/auth"
	"mu/internal/memory"
)

// memoryCard renders the Memory card for /account.
func memoryCard(r *http.Request, acc *auth.Account) string {
	entries := memory.All(acc.ID)
	csrf := htmlpkg.EscapeString(auth.CSRFToken(r))

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>Memory</h4>`)
	b.WriteString(`<p class="text-sm text-muted">What your agent has picked up about you, read back ` +
		`into every question you ask. Delete anything wrong, or anything you would rather it ` +
		`did not keep.</p>`)

	if len(entries) == 0 {
		// Extraction only fires when a message happens to contain a durable
		// fact, so for most accounts this is the state, and "nothing yet" on
		// its own reads as broken. Say what makes something appear.
		b.WriteString(`<p class="text-sm text-muted" style="color:#888">Nothing yet. Say ` +
			`"remember that I'm in London" to an agent and it will show up here, or write ` +
			`one yourself.</p>`)
	} else {
		b.WriteString(`<div class="mem-list">`)
		for _, e := range entries {
			b.WriteString(`<div class="mem-row"><div style="flex:1;min-width:0">` +
				`<span class="mem-key">` + htmlpkg.EscapeString(e.Key) + `</span>` +
				`<div class="mem-val">` + htmlpkg.EscapeString(e.Value) + `</div>` +
				`<div class="mem-when">learned ` + htmlpkg.EscapeString(TimeAgo(e.CreatedAt)) + `</div></div>` +
				`<form method="POST" action="/account" style="margin:0">` +
				`<input type="hidden" name="_csrf" value="` + csrf + `">` +
				`<input type="hidden" name="forget" value="` + htmlpkg.EscapeString(e.Key) + `">` +
				`<button type="submit" class="link-button danger">Forget</button></form></div>`)
		}
		b.WriteString(`</div>`)
	}

	// Written by hand as well as extracted. An agent can call memory_set, so
	// somebody looking at the same list should be able to add to it.
	//
	// Written as a sentence, because the storage shape is a key and a value and
	// nobody is thinking in key-value pairs on this page. Two bare boxes reading
	// "location" and "London" are a puzzle: they are the same width, they have
	// no labels, and nothing says which one is the name of the thing and which
	// one is the thing. Wrapping them in "Remember that my … is …" says it
	// without a single word of explanation, and it matches the sentence the
	// empty state just told you to say to an agent.
	b.WriteString(`<form method="POST" action="/account" class="mem-add">` +
		`<input type="hidden" name="_csrf" value="` + csrf + `">` +
		`<input type="hidden" name="remember" value="1">` +
		`<span class="mem-word">Remember that my</span>` +
		`<input name="key" required maxlength="40" placeholder="location" class="mem-in" aria-label="What to remember about you">` +
		`<span class="mem-word">is</span>` +
		`<input name="value" required maxlength="300" placeholder="London" class="mem-in mem-in-wide" aria-label="What it is">` +
		`<button type="submit">Add</button></form>`)

	if len(entries) > 0 {
		b.WriteString(`<form method="POST" action="/account" style="margin:10px 0 0" ` +
			`onsubmit="return confirm('Forget everything it has learned about you?')">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
			`<input type="hidden" name="forget_all" value="1">` +
			`<button type="submit" class="link-button danger">Forget everything</button></form>`)
	}

	b.WriteString(memoryCardCSS + `</div>`)
	return b.String()
}

const memoryCardCSS = `<style>
.mem-list{display:flex;flex-direction:column;gap:8px;margin:10px 0 0}
.mem-row{display:flex;align-items:center;gap:12px;border:1px solid var(--border-color,#eee);border-radius:8px;padding:10px 14px}
.mem-key{font-weight:600;font-size:14px}
.mem-val{font-size:14px;margin-top:2px}
.mem-when{font-size:12px;color:var(--text-muted,#999);margin-top:3px}
.mem-add{display:flex;flex-wrap:wrap;gap:8px;align-items:center;margin:12px 0 0}
.mem-word{font-size:14px;color:var(--text-muted,#666);white-space:nowrap}
.mem-in{padding:7px 10px;border:1px solid var(--border-color,#d1d5db);border-radius:6px;font-size:14px;font-family:inherit;flex:0 1 130px;min-width:0}
.mem-in-wide{flex:1 1 200px}
</style>`
