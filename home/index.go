package home

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mu/account"
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
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
	// Signed in, this is not your page.
	//
	// It went back and forth twice and the second answer is the right one. The
	// argument for keeping one page in two states was that signing in should
	// not move you somewhere else, and a question half-typed in the box should
	// survive it. Both true, and both smaller than what it cost: a landing page
	// is written for somebody deciding whether they want this, and showing it
	// to somebody who already has an account means the app's front door is an
	// argument aimed at a stranger.
	//
	// On a phone it was plainer than that. The app has a tab bar with Home in
	// it — you press Home and you know where you are — and landing on a
	// wordmark with a marketing tagline over a search box, no rail, no tabs,
	// is a different product opening under you. Folding this page into the app
	// shell instead was the other thing tried, and that made every app page
	// carry a landing page's furniture.
	//
	// So: two pages for two audiences. Out here, a landing. Signed in, /home,
	// which has the rail, the tabs, your inbox and the same box to type in.
	if _, acc := auth.TrySession(r); acc != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	// Its own shell, and that is the point of it.
	//
	// Not the app shell. This page has no rail and should not grow one; a
	// landing with a hamburger on it is the app wearing a landing page's
	// clothes, and on a phone it came with a tab bar as well.
	page := app.RenderIndex(app.Index{
		// What it is, not what to think of it. This said "A network for
		// humans, agents and services" with a paragraph of positioning under
		// it — a claim a stranger is invited to weigh, which is a landing
		// page's job and not a server's.
		Title:       "Micro",
		Description: "A personal assistant you can reach from anywhere: the web, a text, WhatsApp, mail, or a program. Open source and self-hostable.",
		// One name, everywhere.
		//
		// This derived a name from the hostname for a while — micro.mu reading
		// as "Micro" — on the argument that Mu is what you run rather than what
		// you arrived at, so our name on somebody else's front door is the same
		// fault as the pricing copy that used to ship in every binary.
		//
		// The argument is real and the fix was in the wrong place. The wordmark
		// was the only surface it changed: the browser tab still said Mu, the
		// manifest still said Mu, and the app still installed as Mu with a Mu
		// icon. Four surfaces, two names, and nothing explaining the relation —
		// which is worse than either answer on its own.
		//
		// So one name, and the self-hosting concern belongs where it can be
		// answered properly: a setting an operator sets once that moves the
		// wordmark, the title and the manifest together. Deriving a different
		// name for one element out of a hostname is not that.
		// No wordmark in the chrome. It goes on the page, directly above the
		// box — see indexBody.
		TopRight: topRight(),
		Body:     indexBody(),
		Footer:   app.FooterLinks(),
		Tail:     workerScript(),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The same revalidation every other page gets — see app.Respond. This page
	// writes its own response because it does not go through the app shell, and
	// a page with no cache headers is one the browser holds for as long as it
	// likes.
	w.Header().Set("Cache-Control", "no-cache, private")
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
func indexBody() string {
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
	// The name, directly above the box.
	//
	// It was a line in the top left, which is where a name goes on a site with
	// a page under it. There is no page under this one any more — a box, a
	// date and two sentences — so a name in a corner is a label on an empty
	// room, floating a long way from the only thing you are here to use.
	//
	// Above the box and slightly larger, so the two read as one object: this is
	// the thing, and that is where you talk to it. Everything below it is
	// centred on the same axis, which is what makes a page with almost nothing
	// on it look composed rather than unfinished.
	// And what it is, under the name.
	//
	// The wordmark stood alone over a box, which tells somebody arriving what
	// this is called and not what it is. "Mu" is three characters of Greek and
	// carries nothing; the <title> and the meta description both said personal
	// assistant and neither is on the page. So the one line that answers the
	// question, directly under the name, quiet enough that the name is still
	// the name.
	return `<div class="lwrap">` +
		`<div class="lbrand">Micro</div>` +
		`<div class="lwhat">A personal assistant</div>` +
		app.ChatComponent(app.ChatConfig{
			Ask:             true,
			HideSuggestions: true,
			Placeholder:     "What do you need?",
			// Who answers, for the byline over the reply. The default agent,
			// which is what an unpicked box reaches — see agent.DefaultName.
			AgentName: agent.DefaultName(),
			// No row of doors.
			//
			// It was nine links under the box — Search — Archive · News · Video
			// · Social · Markets · Weather · Places · Web — on the argument that
			// a box which answers everything, alone on a page, is a thing you
			// have to go through, and everything the agent does you should be
			// able to do yourself. That argument is still true and it is not an
			// argument for putting the list here: the same page also has to be
			// the quietest thing this product owns, and nine links is the
			// busiest element on it.
			//
			// The services are on /about and in the catalogue, and a signed-in
			// person has them in the rail. What a stranger needs from this page
			// is somewhere to type.
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
			// And no read-aloud. Everybody on this page is signed out — see
			// Index — and the first screen somebody sees is a wordmark, a box
			// and a day. A checkbox about how answers are delivered is
			// furniture in front of somebody who has not asked anything yet.
			Speak: false,
		}) +
		today("") + `
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
/* One centred stack: the name, the box, the day, the brief.
 *
 * It was left-aligned against a 760px edge, which is right for a page with a
 * document on it — six elements of different weights sharing one edge instead
 * of six ragged centres. There is no document now. A box, a date and two
 * sentences hung off a left edge is three things pinned to the side of an empty
 * screen, and the axis that held them together has nothing left to hold.
 *
 * So it centres, and it stays narrow: a short block centred in a wide column
 * has the ragged-edge problem the axis was avoiding, and the fix is a measure
 * the eye can take in without tracking. */
.lwrap{padding:0;max-width:560px;margin:0 auto;width:100%;text-align:center}
/* Slightly larger than the box's own text, so the pair reads as one object —
   this is the thing, that is where you talk to it. */
.lbrand{font-size:2rem;font-weight:800;letter-spacing:-1px;margin:0 0 6px;line-height:1}
/* What it is, under what it is called. Grey and small: it is a caption on the
   name, not a tagline arguing for anything — the moment it is dark enough to
   read as a pitch this page is a landing again.

   The 18px that used to be under the wordmark is under this instead, so the
   gap between the name and the box is unchanged and the two lines sit together
   as one block. */
.lwhat{color:#888;font-size:14px;margin:0 0 18px;line-height:1.3}
/* Today, under the box. */
.ltoday{margin:30px 0 0}
/* The day, in the page's quietest voice. All caps and letter-spaced because it
   is a label rather than a sentence: it says when, and gets out of the way of
   the thing that says what. */
.lday{display:block;margin-bottom:10px;font-size:11px;
  text-transform:uppercase;letter-spacing:.09em;color:#aaa}
.lrow{margin:0;line-height:1.6}
/* A labelled group, separated from the one above it. Just enough that the eye
   finds the seam — this is still one block, not three sections. */
.lgroup{margin-top:18px}
.lgroup-label{display:block;margin-bottom:5px;font-size:10px;
  text-transform:uppercase;letter-spacing:.09em;color:#bbb}
/* The brief is the one thing here worth reading, so it gets the size — and it
   is two sentences, which is short enough to centre without the ragged edge
   that makes centred prose hard to track. */
.lbrief{font-size:15px;color:#444;line-height:1.65}
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

// topRight is the corner: the way in, and nothing else.
//
// One link. It was Sign up and Log in, on the argument that a stranger here is
// deciding whether to join and somebody returning already decided — which is
// true and put two controls in a corner whose whole job is to be the one thing
// you can do from here. The login page offers signing up on it, so nothing is
// lost but a fork in front of somebody who has not asked for one.
//
// Nothing else has ever earned this slot. Install app stood here for a while
// and appeared on only some browsers, saying nothing about what state you are
// in or what to do about it, which is the corner's entire purpose.
// topRight is the landing's corner: the way to have an account, and the way
// back to one.
//
// The same pair the app shell draws — see app.headCorner, which carries the
// reasoning and the invite-only exception. Written out here rather than called,
// because this page has its own shell and its own stylesheet: the corner in
// mu.css is #head-out and this one is .login-link, and the markup differs by
// the wrapper each of them needs.
//
// No redirect on the way in. The landing is the one page where signing in
// should move you somewhere else, and it already does.
func topRight() string {
	signup := ""
	if !auth.InviteOnly() {
		signup = `<a class="primary" href="/signup">Sign up</a>`
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
	// The brief, and nothing beside it.
	//
	// This carried a row of market tickers and the day's image as well. Held
	// against the thing this competes with, that is indefensible: write to
	// agent@ and what comes back is an answer, with no ticker under it and no
	// picture attached. The web front door had accumulated four-colour price
	// movements and a photograph on a page whose whole argument is that you get
	// what you need and leave.
	//
	// Both still exist for somebody who asked to see them — /markets, /images,
	// and the cards on Home, which is a dashboard because a signed-in person
	// went looking for one. A stranger did not.
	rows := []string{
		briefRow(viewerID),
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
	// The day, then what happened. No heading over it.
	//
	// It said "Morning brief" / "Afternoon brief" above the date, which is a
	// label on the only block there is — and naming a thing that has nothing to
	// be distinguished from is furniture. The date says when, the brief says
	// what, and between them is the weather where the reader has said where
	// they are.
	now := account.LocalNow(viewerID)
	return `<div class="ltoday" data-brief>` +
		`<span class="lday">` +
		html.EscapeString(now.Format("Monday, 2 January")) +
		weatherBit(viewerID) + `</span>` +
		b.String() + `</div>`
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

// title is a service's name as a heading would write it. The registry keys are
// lower case because they are identifiers.
func title(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
