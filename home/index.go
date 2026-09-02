package home

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mu/account"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/images"
	"mu/service/markets"
	"mu/service/weather"
)

// Index is the front door for anyone not signed in: something to try, then
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
// guest gets a few agent queries a day (agent/guest.go) against every service
// that is not somebody's private data, which is enough to watch it read the
// news, check a market and answer.
//
// The copy used to name the number — "three questions a day without an
// account" — and that was the wrong promise to make. There is no free plan
// here, and a page offering a daily allowance is describing one; it also fixed
// in writing a number that is an operator's to change, and made a spend that
// scales with arrivals sound like a feature. What this is, and what it now
// says, is a demonstration: enough to see it work, bounded by a ceiling the
// operator sets, and not a tier anybody can plan around.
//
// The description still follows. It reads better as a caption than as a pitch.
func Index(w http.ResponseWriter, r *http.Request) {
	var viewerID string
	if _, acc := auth.TrySession(r); acc != nil {
		viewerID = acc.ID
	}
	body := indexBody(viewerID)

	page := app.RenderIndex(app.Index{
		// What it is, not what to think of it. This said "A network for
		// humans, agents and services" with a paragraph of positioning under
		// it — a claim a stranger is invited to weigh, which is a landing
		// page's job and not a server's.
		Title:       "Mu",
		Description: "A personal server: mail, chat, files, an inbox with an address, and an agent that reaches its tools. Open source and self-hostable.",
		// The name, and what it is for. Markup rather than the shell's own
		// tagline slot, because that renders a separate block underneath and
		// this belongs on the same line — see .btag.
		Brand: `Mu <span class="btag">a personal assistant</span>`,
		// No tagline in the chrome. This slot held "An Inbox for Agents" — the
		// line this positioning replaced — sitting directly above a headline
		// that said something else, and swapping it for the new line only made
		// the page say the headline twice in three centimetres. The
		// headline is where the line belongs: it is set at 38px and the tagline
		// slot is 18px, so the chrome copy was a smaller, duplicate version of
		// the thing immediately below it. Nothing renders the two together
		// except the page, which is why neither reading caught it.
		// Two links, and which two says whether anybody is signed in. That is
		// the only thing on this page that does. See topRight.
		TopRight: topRight(viewerID),
		Body:     body,
		Footer:   app.FooterLinks(),
		Tail:     workerScript(),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page)) //nolint:errcheck
}

// indexBody is the page itself, separate from serving it.
//
// Separated so it can be read by a test, which is how the duplicated guest
// note went unnoticed: nothing could look at the whole page at once, so two
// paragraphs saying the same thing stayed correct individually and wrong
// together.
// One screen, and it fits on one.
//
// This page had a chat box, three feature cards and a four-step section on
// pointing an MCP client at the endpoint — a landing that argued for four
// things and scrolled for two of them. None of the parts were wrong; each was
// true and each argued for something different. A visitor here is deciding
// whether they want an agent at all, which is one question, and how to
// configure Cursor is a question asked by somebody who already decided. That
// one belongs on /tools, which is the page about tools.
//
// What is left: a headline, a sentence, the address, one way on.
//
// There were two buttons and the second kept being the wrong one. It was
// "Browse the tools", which answers a question a first-time visitor has not
// asked; then "Browse the agents", which sends somebody signed out to a page
// that lists the agents they have not made. A second button is a choice, and
// there was only one thing to do here.
//
// And it has to actually fit, which is a measurement rather than an intention.
// The first version of this centred the block with min-height:calc(100vh -
// 200px), forgetting that the shell it sits in already pads 14vh from the top
// and that there is a wordmark above and a footer below — so the page asked for
// a full screen inside something that had already spent a fifth of one, and
// scrolled by 74px at 1440x900 while claiming to be one screen. The block sizes
// to its content now and the shell does the positioning.
//
// Not centred in the leftover space either, which was the next thing tried: the
// shell pads 14vh before the wordmark, so centring what comes after it leaves
// the wordmark alone at the top with a hole under it. The group reads as one
// thing when it flows, and the whitespace goes at the bottom where a landing
// normally puts it.
//
// The prose explaining all that is here rather than in the stylesheet, because
// a comment inside the <style> block is served to every visitor — which is how
// a test looking for "Connect via MCP" on the page found it in the note saying
// the section had been removed.
// No address on the page.
//
// It has been through every position: the largest element on the page, then a
// retyping animation, then a static line under the button. Each move was an
// attempt to say "and you can email it too" without the page becoming about
// email. The move that works is not making it smaller — it is that a landing
// gets one call to action, and an address printed beside a button is a second
// one that nobody signed out can use. You cannot write to you+agent@ until
// there is a you.
//
// The lead still says you can write to it from anywhere, which is the fact. The
// address is on the pages where it is actionable — /agents, Connect, the inbox.
// indexBody is the signed-out page: a search box.
//
// # Why there is no pitch here any more
//
// This was a landing page — a headline, a paragraph of positioning, Get
// started, and "The agent is free. Tools that cost us are priced." That last
// line is our business model, and it shipped in every binary: on somebody
// else's server they pay the vendor themselves, so "costs us" is us, in their
// house, describing an arrangement they are not part of.
//
// The general fault is bigger than that line. A landing page is for a stranger
// who has not decided yet, which is a real audience for micro.mu and nobody at
// all on an instance that one person installed and is already logged into on
// their other device. Marketing is a thing an instance may serve; it is not a
// thing the software should contain.
//
// # A search box, because the archive is public
//
// What is useful without an account is what this instance has collected — and
// /archive is already public by construction: an entry with an owner is never
// returned from it, so there is nothing here to leak and nothing to gate. The
// box is the same one Home shows when no model is configured, pointed at the
// same page, for the same reason: you type what you are looking for and
// something finds it.
//
// That is also the whole of what a utility's front door is. nginx's default
// page says it is nginx and that it is working. This says what it is, that it
// is running, and gives you something to do with it.
func indexBody(viewerID string) string {
	// The agent, not a search box.
	//
	// This searched the archive, and the reason was that search is the half
	// that works with no model — which is a constraint, and it got dressed up
	// as a thesis. The README has said the right order the whole time:
	// "services and the archive become tools for agents to use". The archive is
	// context. The agent is the door.
	//
	// Where there is no model the same box still takes what you type, searches
	// the archive and says why — see app.ChatComponent, which degrades rather
	// than becoming a different product depending on configuration.
	//
	// # And it is not the only door
	//
	// A box that answers everything, with nothing else on the page, is a thing
	// you have to go through. The links under it go straight to the services —
	// the archive, the news, the video — so anything the agent would fetch is
	// also one click away without asking it. That is the property that keeps it
	// a tool: everything it does, you can do yourself.
	return `<div class="lwrap">` +
		app.ChatComponent(app.ChatConfig{
			Ask:             true,
			HideSuggestions: true,
			Placeholder:     "What do you need?",
			// Directly under the input, which is why the component draws it
			// rather than this page writing it afterwards — written after, it
			// lands below the Speak toggle. See ChatConfig.Doors.
			Doors: directDoors(),
			// No agent picker here, signed in or not.
			//
			// Not because it would not work — signed in it would — but because
			// choosing which of your agents answers is not a front-door
			// question. This page is for arriving and asking one thing; picking
			// a specialist first is the opposite motion, and it belongs where
			// the rest of the customised surface is. Home has it.
			//
			// Signed out it would be an empty select besides.
			OfferAgentPicker: false,
			// And no read-aloud for a stranger.
			//
			// A guest does get a real answer here — that part of the old
			// reasoning had expired and the comment on ChatConfig.Speak now says
			// so. This is a different reason: the first screen somebody sees is
			// a wordmark, a box and a day, and a checkbox about how answers are
			// delivered is furniture in front of somebody who has not asked
			// anything yet. Signed in, they have.
			Speak: viewerID != "",
		}) +
		today(viewerID) + `
</div>

<style>
/* One axis, and one measure.
 *
 * Everything on this page was centred in a 640px column on a 1280px screen —
 * so two thirds of the viewport was empty and the container could only grow
 * downward. Every element added made the page taller rather than fuller, which
 * is why each new one felt like it did not fit.
 *
 * And six things of different weights centred against each other — a heading,
 * a small-caps date, prose, tabular numbers, a picture, a caption — have a
 * ragged edge on both sides. Nothing lines up with anything, so they read as
 * fragments rather than as one thing.
 *
 * Left, against a single edge, at the measure every other page uses. The block
 * is still centred in the screen; its contents are not centred in the block. */
.lwrap{padding:0;max-width:760px;margin:0 auto;width:100%;text-align:left}
/* The wordmark keeps the block's left edge, and says what this is.
 *
 * It was centred for a moment on the argument that a wordmark over a search box
 * is the shape everybody knows. True, and it costs the one thing the axis
 * bought: a name floating over a left-aligned document reads as a header from a
 * different page. The better answer is to give the left edge something worth
 * holding — the name, and then in a smaller face what the name is for.
 *
 * A stranger arriving here has never been told what this is. The page has a
 * box, a row of services and a brief, all of which are evidence and none of
 * which is a sentence. Four words next to the name is the whole explanation,
 * and it costs a line that was empty anyway. */
.index-page .brand{width:100%;max-width:760px;text-align:left;
  display:flex;align-items:baseline;gap:12px;flex-wrap:wrap}
/* Baseline-aligned, not centred on the cap height: the two sit on one line and
   the eye reads the smaller one as a continuation rather than as a label. */
.btag{font-size:15px;font-weight:400;letter-spacing:0;color:#888}
/* Today, under the box. Three rows, each one line, and the block does not
   scroll — see today() for why this is not a grid of cards. */
.ltoday{margin:34px 0 0}
/* The name of the thing, then the date under it. Set as a heading because it
   is one — this block is the brief, and everything below it belongs to it. */
.lbrief-head{margin:0 0 3px;font-size:15px;font-weight:600;color:#333}
.lday{display:block;margin-bottom:12px;font-size:11px;
  text-transform:uppercase;letter-spacing:.08em;color:#aaa}
.lrow{margin:0;line-height:1.6}
/* A labelled group, separated from the one above it. Just enough that the eye
   finds the seam — this is still one block, not three sections. */
.lgroup{margin-top:18px}
.lgroup-label{display:block;margin-bottom:5px;font-size:10px;
  text-transform:uppercase;letter-spacing:.09em;color:#bbb}
/* The brief is prose and is set as prose. It is the one thing here worth
   reading rather than clicking, so it gets the size and the measure. */
.lbrief{font-size:15px;color:#444}
.lbrief a{color:#444;text-decoration:underline;text-underline-offset:2px}
/* Numbers, so they are set as numbers: tabular, quiet, one line. */
.lmarkets{font-size:13px;font-variant-numeric:tabular-nums}
.lmarkets a{color:#777;text-decoration:none}
.lmarkets a:hover{color:#555}
/* Direction is a colour, so the row can be read without reading it. Muted
   rather than a traffic light: this is a glance, not an alarm. */
.lup{color:#2e7d32}
.ldown{color:#b3261e}
/* The dot between two things on one row. It was defined with the headlines and
   went with them when those became a list, leaving the markets row joining
   "-2.1%" to "Tesla" with an unspaced dot. */
.lsep{margin:0 7px;color:#ccc}
/* The day's picture, last. Sized to the block rather than to itself, so a
   provider that changes its dimensions cannot change the page. */
.limage a{display:block;text-decoration:none;color:inherit}
/* A band, not a poster. The image arrives square and at the block's full width
   it is 580px tall — taller than everything above it put together, which
   inverts the page: the brief is the point and the picture is the flourish.
   Cropped to a band it finishes the page instead of becoming it, and the page
   still ends on one screen. */
.limage img{width:100%;height:150px;object-fit:cover;border-radius:8px;display:block}
.lcap{display:block;margin-top:6px;font-size:11px;text-transform:uppercase;
  letter-spacing:.08em;color:#bbb}
</style>`
}

// topRight is the corner: the way in, or the way deeper in and the way out.
//
// Two links either side of the line. Signed out, Sign up and Log in, in that
// order — the first is what a stranger on this page is deciding about and the
// second is for somebody who already decided. Signed in, Home and Log out:
// Home is the dashboard, which is where the rail of your inbox and agents and
// balance lives, and Log out is here because this page has no rail to put it
// in.
//
// Sign up only where signing up is a thing this instance allows. On an
// invite-only one it is a door that opens onto a form asking for a code, which
// is worse than no door.
//
// # No Install app
//
// It stood here on the argument that it is not a destination — the same page,
// made resident — and that is still true and is not the point. The corner is
// where somebody looks to find out what state they are in and what they can do
// about it, and a third control that appears only on some browsers, saying
// something about neither, made a two-word answer into a three-word one. The
// page is still installable; browsers offer it in their own menus, which is
// where a browser feature belongs.
func topRight(viewerID string) string {
	if viewerID != "" {
		return `<a href="/home">Home</a><a href="/logout">Log out</a>`
	}
	signup := ""
	if !auth.InviteOnly() {
		signup = `<a href="/signup">Sign up</a>`
	}
	return signup + `<a href="/login">Log in</a>`
}

// today is what you are given for arriving, before you ask anything.
//
// # Why the front door has content at all
//
// The page argued that you can get what you need and leave, and made the
// argument with a text box — which is a promise a stranger has to spend
// something to test. This is the claim demonstrated instead: the date, what
// happened, where the markets went. Read in fifteen seconds, finished, and then
// you are done. The box is there for the times that is not enough.
//
// # And not the headlines
//
// Three of them were here, under their own heading, and they were the brief
// again. agent/brief writes that sentence *from* the headlines — see its
// sources — so the page was showing the working and the answer, one above the
// other, and the answer is the better of the two because somebody judged it.
// A front page that prints its own source material is a feed with a summary at
// the top of it.
//
// # Why it is rows and not cards
//
// Because the thing being avoided is a portal. Cards are the dashboard idiom
// and /home is the dashboard: it has a rail of your inbox, your agents and your
// balance, and a grid of sixteen services, and it is the right page for
// somebody who came to look at things. This page is for somebody who came to
// find one thing out. The moment it grows a second column it is /home with
// worse navigation, and there is no reason for two of those.
//
// So: rows, each one line, and a hard rule that nothing here scrolls.
//
// # It costs nothing to draw
//
// Every line is read from something already in memory. agent/brief writes its
// sentence on a timer for the whole instance; markets and news are fetched by
// their own services for the cards. Nothing on this page calls a model, which
// is what makes it safe on the one page that takes arbitrary traffic.
//
// # Signed in, it is the same page with your day in it
//
// The brief gains the clauses that need an account — what arrived, what your
// agents have in hand, what is owed, what is on today — and everything else is
// identical. That is the whole difference between the two states of this page,
// and it is deliberately the entire pitch for signing up: the world's day is
// the same for everybody and yours is not.
func today(viewerID string) string {
	rows := []string{
		briefRow(viewerID),
		group("Markets", marketsRow()),
		imageRow(),
	}

	var b strings.Builder
	for _, row := range rows {
		if row != "" {
			b.WriteString(row)
		}
	}
	if b.Len() == 0 {
		return ""
	}

	// data-brief on the block, so asking a question takes the whole of today
	// off the page rather than pushing it under the answer. All of this is what
	// you are told without asking; an answer is what you get when you do, and
	// they are not both on the screen at once. See hideBrief in
	// app.ChatComponent.
	//
	// Dated once, at the top, rather than per row. Three timestamps on three
	// lines is a page about its own freshness.
	now := account.LocalNow(viewerID)
	return `<div class="ltoday" data-brief>` +
		`<h2 class="lbrief-head">` + html.EscapeString(briefName(now)) + `</h2>` +
		`<span class="lday">` +
		html.EscapeString(now.Format("Monday, 2 January")) +
		weatherBit(viewerID) + `</span>` +
		b.String() + `</div>`
}

// group puts a faded heading over a row, and nothing at all over an empty one.
//
// The three rows ran together as one block of decreasing font size, so the
// prices read as a footnote to the brief and the headlines as a footnote to the
// prices. A word over each says what it is and, more usefully, says where one
// thing stops. Small caps and grey, the same treatment Home gives its sections
// — see sectionRule — so the two pages label things the same way.
//
// The brief has no heading. It is the answer to "is there anything I need to
// know", it sits directly under the date, and "Brief" over one sentence is a
// caption on a caption.
func group(label, row string) string {
	if row == "" {
		return ""
	}
	return `<div class="lgroup"><span class="lgroup-label">` +
		html.EscapeString(label) + `</span>` + row + `</div>`
}

// briefName is what to call the brief at this hour.
//
// Morning, Afternoon, Evening — and the hour is the reader's, not the server's.
// account.LocalNow reads the zone they set with their place, so an instance in
// Virginia does not wish somebody in Tokyo good morning at nine at night. With
// no place set it is the machine's clock, which on a self-hosted install is the
// same room as the person.
//
// Not "Daily Brief". The line under it is already dated, and a word that says
// which part of the day it is says something the date does not: the same brief
// read at eight and at six is a different thing to the person reading it.
//
// The boundaries are the ordinary ones and nothing turns on them being exactly
// right — five in the afternoon is arguably still afternoon, and calling it
// evening costs nobody anything.
func briefName(now time.Time) string {
	switch h := now.Hour(); {
	case h < 12:
		return "Morning brief"
	case h < 17:
		return "Afternoon brief"
	}
	return "Evening brief"
}

// weatherBit is the temperature where you are, beside the date.
//
// Only for somebody signed in who has told us where they are — /account keeps a
// place, and account.PlaceLine is what the agent already reads. Nothing here
// asks: a permission prompt on the first screen a stranger sees is the highest
// friction there is, and it is the move of a site that wants something from you
// before it gives you anything.
//
// From the cache only, so the page never waits on a third party. See
// weather.Now. The fetch that fills it is kicked off behind the answer, so the
// line is there next time rather than never.
//
// Beside the date because that is what it is: the other half of "what is today
// like". Two facts, one line, and neither is worth a row of its own.
func weatherBit(viewerID string) string {
	if viewerID == "" {
		return ""
	}
	lat, lon, ok := account.PlaceOf(viewerID)
	if !ok {
		return ""
	}
	temp, desc, hit := weather.Now(lat, lon)
	if !hit {
		go weather.Warm(lat, lon)
		return ""
	}
	out := ` · ` + strconv.Itoa(temp) + `°C`
	if d := strings.TrimSpace(desc); d != "" {
		out += ` ` + d
	}
	return html.EscapeString(out)
}

// imageRow is the day's picture, last.
//
// Last because it is the one thing here you look at rather than read, and
// because it is the only row that is not information — it is the instance
// having made something today. A page that ends on it ends on a full stop
// rather than trailing off into more text.
//
// No heading over it. Every other row needs a word to say what the numbers are;
// a picture does not, and the caption under it says the rest more quietly than
// a label above would.
//
// Nothing at all on a day it has not run, the same as every other row here.
func imageRow() string {
	url, theme, ok := images.Today()
	if !ok {
		return ""
	}
	caption := "Daily image"
	if t := strings.TrimSpace(theme); t != "" {
		caption += " · " + t
	}
	return `<div class="lgroup limage"><a href="/images">` +
		`<img src="` + html.EscapeString(url) + `" alt="` + html.EscapeString(caption) + `" loading="lazy">` +
		`<span class="lcap">` + html.EscapeString(caption) + `</span></a></div>`
}

// briefRow is what happened, in a sentence.
//
// Signed out that is the instance's line about the world; signed in it is that
// plus the clauses about you, from the same function Home's card uses. See
// briefParts — one place decides what a brief says, two places decide how it is
// set.
func briefRow(viewerID string) string {
	parts := briefParts(viewerID)
	if len(parts) == 0 {
		return ""
	}
	// The clauses carry their own markup — waiting() links the inbox, owed()
	// links the wallet — so this is not escaped. briefParts is the boundary,
	// and everything reaching it escapes its own text. See home/brief.go.
	return `<p class="lrow lbrief">` + strings.Join(parts, " ") + `</p>`
}

// marketsRow is where the money went, in one line.
//
// Four fixed names — BTC, ETH, OIL, GOLD — and not the biggest movers. The
// movers are three coins every time, because coins move most; one mover from
// each kind gives "Wheat, Tesla, SOL", which is three things a reader has to
// work out the reason for. These four do not change, so somebody learns where
// each sits on the row and reads it in a glance. See markets.Spread.
func marketsRow() string {
	quotes := markets.Spread(4)
	if len(quotes) == 0 {
		return ""
	}
	var parts []string
	for _, q := range quotes {
		sign := ""
		if q.Change24h >= 0 {
			sign = "+"
		}
		cls := "ldown"
		if q.Change24h >= 0 {
			cls = "lup"
		}
		parts = append(parts, html.EscapeString(q.Name)+" "+
			html.EscapeString(price(q.Price))+
			` <span class="`+cls+`">`+
			html.EscapeString(fmt.Sprintf("%s%.1f%%", sign, q.Change24h))+`</span>`)
	}
	return `<p class="lrow lmarkets"><a href="/markets">` +
		strings.Join(parts, `<span class="lsep">·</span>`) + `</a></p>`
}

// price is a number somebody reads rather than reconciles.
//
// No decimals above a hundred — nobody glancing at a page needs to know
// bitcoin is at 77551.38 rather than 77551 — and two below it, where the
// pennies are the whole movement.
func price(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

// workerScript registers the service worker on the front door.
//
// This was the tail of installScript, which has gone with the Install button.
// The registration is not about that button and never was: a browser will not
// offer to install a site that has no worker, the worker is what handles a push
// notification when nothing is open, and the app shell registers it on every
// other page. Without this the first page a visitor sees is the one page it is
// never registered from — which is the exact bug the script was written to fix,
// and which removing the button reintroduced. TestTheServiceWorkerStillRegisters
// is what caught it.
func workerScript() string {
	return `<script>
(function () {
  if (!navigator.serviceWorker) return;
  // updateViaCache:'none', the same as the app shell — see internal/app. The
  // default consults the HTTP cache for the worker script, which is how a
  // device ends up running a months-old copy.
  navigator.serviceWorker.register('/mu.js', {scope: '/', updateViaCache: 'none'})
    .then(function (reg) { if (reg && reg.update) reg.update(); })
    .catch(function () {});
})();
</script>`
}

// directDoors is the row of doors directly under the box.
//
// Under the box, not at the foot of the page. Every one of these is a search,
// the thing above them is a search box, and the row reads as the second half of
// the same sentence: ask anything here, or search one of these. At the bottom,
// under the day, it read as a footer — a list of other places to go, which is
// what it used to say and the least interesting thing about it.
//
// Every service a signed-out visitor can open came to twenty-one names — a
// paragraph of them, wrapping onto two lines, including Browser and Text and
// Users, which are tools rather than places anybody arrives wanting. A list
// that long is not a set of doors, it is a wall with the doors drawn on it.
//
// The set and its order are service.Guest's, which is also the set of tools a
// guest's question can reach. One list, so a door the page offers is always a
// door the agent can walk through — two would be one that had drifted, and the
// drift would be invisible from either side.
//
// Everything else stays reachable: /tools lists all of them and the footer
// links it.

func directDoors() string {
	page := map[string]string{}
	for _, spec := range service.Specs() {
		if spec.Page == "" {
			continue
		}
		page[strings.ToLower(spec.Name)] = spec.Page
	}

	var links []string
	for _, name := range service.Guest() {
		href, ok := page[name]
		if !ok {
			continue
		}
		links = append(links, `<a href="`+html.EscapeString(href)+`">`+
			html.EscapeString(title(name))+`</a>`)
	}
	if len(links) == 0 {
		return ""
	}
	// "Search" and not "Or go straight there". Every one of these is a search
	// over a different set — the archive, the news, the video, the shops — and
	// the old line said only that they were somewhere else to go, which is the
	// least interesting thing about them. Naming the verb says what the row is
	// for and, next to a box that also searches, says what the difference is:
	// the box asks anything, these each ask one thing.
	return "Search — " + strings.Join(links, " · ") + "."
}

// title is a service's name as a heading would write it. The registry keys are
// lower case because they are identifiers.
func title(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
