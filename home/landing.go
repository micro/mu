package home

import (
	"net/http"
	"strconv"

	"mu/internal/api"
	"mu/internal/app"
)

// Landing is the front door for anyone not signed in: something to try, then
// what this is and how to connect to it.
//
// It used to be three pages. The live home was the front door and said nothing
// about what Mu is; /about carried the pitch; /agents carried a second pitch
// plus the payment explanation, and /tools linked to it — so finding out what
// this place is took three hops through pages that each looked like a landing.
// They are one page now, and /about and /agents redirect here.
//
// And then it was one page that only described things. Three buttons, three
// cards and a setup guide: everything about the tools, nothing you could do
// with them. "See it working" was a link, which is a promise rather than a
// demonstration.
//
// The model to copy is ollama. Its product is running models locally, and what
// makes that land is `ollama run llama3` putting you in a conversation on the
// first command — because nobody wants a model, they want to use one. Nobody
// wants tools either. So the tools are used here, on arrival, by anyone: a
// guest gets three agent queries a day (agent/guest.go) against every service
// that is not somebody's private data, which is enough to watch it read the
// news, check a market and answer.
//
// The description still follows. It reads better as a caption than as a pitch.
func Landing(w http.ResponseWriter, r *http.Request) {
	host := app.BaseURL(r)
	// Counted, not claimed. This said "67 real tools" as a literal and the
	// endpoint was serving 72 by the time anyone checked.
	body := `<p class="lead">` + strconv.Itoa(api.ToolCount()) + ` tools an agent can call — news, web search, mail,
markets, weather, places, storage — behind one account instead of one per provider.
Ask something and watch it use them.</p>

<div class="ltry">` + chatComponent(true) + `</div>

<p class="ltrynote">Three questions a day without an account. <a href="/signup">Sign up</a> for
your own agent, an email address it answers on, and the endpoint your own client can call.</p>

<div class="lctas">
  <a class="lcta" href="/tools">Browse the tools →</a>
  <a class="lcta lcta-alt" href="/mcp">MCP endpoint</a>
</div>

<div class="lcards">
  <div class="lcard"><h3>Real world access</h3><p>The news, the markets, the weather, the web, your mail — the things happening outside a model's training data. And run here, not proxied: this instance is the mail server, the feed aggregator, the search index and the sandbox.</p></div>
  <div class="lcard"><h3>One endpoint, not nine</h3><p>News, search, mail and storage for an agent usually means wiring up a server for each. This is one connection and one set of credentials.</p></div>
  <div class="lcard"><h3>Run anywhere</h3><p>Build an agent here and this instance runs it, or point your own at the endpoint and run it wherever you like — same tools either way. And it is one Go binary: self-host it and anyone paying to call your tools pays you.</p></div>
</div>

<div class="lpay" id="connecting">
  <h2>Connecting</h2>
  <p class="lnote">Two ways in, depending on what your client reads. Both need an account —
  the same one you sign into the app with.</p>
  <ol>
    <li><b>Cursor, and clients with a config file.</b> Create a token at <a href="/token">/token</a>
    and point them at <code>` + host + `/mcp</code> with an
    <code>Authorization: Bearer</code> header.</li>
    <li><b>Claude Desktop.</b> Settings → Connectors → Add custom connector, and paste
    <code>` + host + `/mcp</code>. It opens a browser and asks you to sign in — no token needed.</li>
    <li><b>Then call anything.</b> Calls draw on your credits — most tools are included, the ones
    that cost us money are priced on <a href="/pricing">pricing</a>.</li>
  </ol>
  <p class="lnote">No account yet? <a href="/signup">Create one →</a> Full setup on
  <a href="/tools">Tools</a>.</p>
</div>

<style>
.lead{max-width:600px;text-align:center;color:#555;font-size:16px;line-height:1.6;margin:0 auto 22px}
.ltry{max-width:660px;margin:0 auto 10px;text-align:left}
.ltrynote{max-width:660px;margin:0 auto 26px;text-align:center;font-size:13px;color:#888}
.ltrynote a{color:#111}
.lead a{color:#111}
.lcards{display:flex;flex-wrap:wrap;gap:14px;max-width:760px;justify-content:center;margin:34px auto 0}
.lcard{flex:1 1 220px;min-width:220px;max-width:240px;border:1px solid #e5e5e5;border-radius:10px;padding:16px 18px;background:#fff;text-align:left}
.lcard h3{margin:0 0 6px;font-size:1em}
.lcard p{margin:0;font-size:14px;color:#666;line-height:1.5}
.lctas{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;margin:0}
.lcta{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:11px 22px;border-radius:8px;font-weight:700;font-size:15px}
.lcta-alt{background:#fff;color:#111;border:1px solid #ddd}
.lcta-alt:hover{border-color:#bbb}
.lpay{max-width:620px;margin:44px auto 0;text-align:left;border-top:1px solid #eee;padding-top:28px}
.lpay h2{font-size:1.15em;margin:0 0 12px}
.lpay ol{margin:0 0 14px;padding-left:20px}
.lpay li{margin:0 0 10px;font-size:15px;line-height:1.55;color:#333}
.lpay code{background:#f4f4f5;border-radius:4px;padding:1px 5px;font-size:.9em}
.lnote{font-size:14px;color:#666;margin:0}
</style>`

	page := app.RenderLanding(app.Landing{
		Title:       "Mu — Tools for Agents",
		Description: "The everyday internet as tools an agent can call — news, web search, mail, markets, weather, video, storage. Over MCP and REST, paid per request. Open source and self-hostable.",
		Brand:       "Mu",
		Tagline:     "Tools for Agents",
		TopRight:    `<a href="/login">Sign in →</a>`,
		Body:        body,
		Footer:      app.FooterLinks(),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}
