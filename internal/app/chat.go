package app

import (
	"encoding/json"
	htmlpkg "html"
	"strings"
)

// JSString returns s as a safely-quoted JavaScript string literal (with
// surrounding quotes) for embedding in inline scripts.
//
// In a <script> block only. The quotes it produces are double quotes, so
// putting one inside a double-quoted HTML attribute ends the attribute — see
// JSAttr, which is for that and exists because this was used there twice.
func JSString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// JSAttr is JSString for an inline handler: onclick="f(<here>)".
//
// The same literal with its quotes HTML-escaped, so the attribute survives.
// The parser turns &#34; back into a quote before the JavaScript is compiled,
// so what runs is exactly what JSString produced.
//
// This exists because the plain one was used in two attributes and both were
// broken in a way nothing catches by reading:
//
//	onclick="muSessionDelete("fe3918b6…",event)"
//
// The attribute ends at the first quote, so the handler is the fragment
// `muSessionDelete(` — a syntax error, and the browser's complaint is
// "Unexpected end of input", which names neither the button nor the id. The
// delete cross on a conversation did nothing at all, and so did + New, for as
// long as both have existed.
func JSAttr(s string) string {
	return htmlpkg.EscapeString(JSString(s))
}

// ChatConfig configures the shared chat component.
type ChatConfig struct {
	// Ask makes this box talk to the agent. Without it, it searches.
	//
	// A default of search rather than of asking, because the pages that want a
	// chat know they do — /agents is a page about an agent, so a box there is
	// obviously for talking to it — while every other embed either wants a
	// search box or has never thought about it, and a search box is the answer
	// that works with no model, no key and no account.
	//
	// The old default was the other way and it produced the bug this fixes: the
	// signed-out page searched, Home asked, so the same control did two
	// different things depending on whether you had signed in.
	Ask bool
	// ContextID seeds the conversation's server-side thread id, so follow-up
	// messages continue the same session. Empty starts a new session.
	ContextID string
	// InitialConvHTML is pre-rendered conversation HTML (prior turns) injected
	// into the log when reopening a session. When set, the component does not
	// restore from sessionStorage.
	InitialConvHTML string
	// HideSuggestions suppresses the component's built-in suggestion pills — used
	// where the host page supplies its own (e.g. Home's personalised chips).
	HideSuggestions bool
	// OfferAgentPicker shows which agent is answering, and lets the reader
	// change it.
	//
	// The component has always sent window.muActiveAgent, and the server has
	// always honoured it — but the only thing that could set it was the rail on
	// /agent. So somebody could create an agent, go to the input on Home, and
	// have no way to reach it: the agent existed and could not be talked to
	// from the place you would naturally talk to it. That is most of why a new
	// agent reads as a toy.
	OfferAgentPicker bool
	// Speak offers to read the answer out loud.
	//
	// Off by default, and the default is the landing page. The box there is the
	// front door: a signed-out ask returns 401 and the component renders a
	// refusal with two ways on — search the archive, or sign in and ask it. So
	// no answer can arrive on that page, and a toggle controlling how one is
	// delivered sat under it anyway, on the first screen a stranger sees.
	//
	// The microphone is not gated with it, and the asymmetry is the point: the
	// mic fills the box, and the box works — what you dictate can be carried to
	// the archive or through a sign-in. It is an input to something that
	// happens. Speak is an output from something that cannot.
	Speak bool
	// Placeholder is the grey text in the empty box, and Suggestions are the
	// pills under it.
	//
	// Both were constants, so every chat on the site read "Try: give me a
	// morning brief" — including the one on the Weather agent's page, which
	// suggests a different agent's work on the page you opened to get away
	// from the general one. Empty keeps the general set, which is right for
	// Home and for a guest: they have not chosen a specialist.
	Placeholder string
	Suggestions []string
	// StorageNS namespaces the component's sessionStorage keys so different
	// surfaces (Home, /agent) keep separate in-tab conversations and never show
	// each other's. When empty the component is ephemeral: it neither restores
	// nor saves, so it always starts clean (used for Home's quick-ask box).
	StorageNS string
	// Pending says this conversation is waiting on an answer that is not in
	// InitialConvHTML, because it had not been written when the page rendered.
	//
	// The case is a refresh mid-run. The run does not stop — it finishes and
	// writes its reply into the record — but the SSE reader died with the old
	// page, so the new one draws the question, nothing after it, and no reason
	// to look again. With this the component shows that it is still working and
	// polls until the answer lands. See agent.Pending.
	Pending bool
	// Transcript makes this a conversation rather than a box.
	//
	// The difference is where the input is. On Home you arrive at a box and
	// type into it: the box is the whole point and it belongs at the top. On
	// /agent you are reading an exchange that already has turns in it, and
	// every chat ever built puts the input under them — because what you are
	// looking at is the last thing said, and what you do next is say the next
	// thing.
	//
	// This was one shape for both. The comment in the script called it reading
	// "top-down", which is a reasonable thing to want and is not what a
	// conversation is: it left the newest turn at the top, the input above it,
	// and everything older stretching away below.
	Transcript bool
}

// ChatComponent returns the single, shared chat UI used everywhere Mu talks to
// the agent — the home assistant, the guest landing page, and the /agent chat
// surface. It is self-contained (one <div id="mu-chat"> with input, suggestion
// pills and a conversation log, plus scoped styles and a small script). The
// script POSTs to /agent and renders the SSE stream inline, tracks the server
// thread id (context_id) so a session continues across turns, and keeps the
// conversation in the DOM + sessionStorage. window.muChatAsk(text)
// submits a query; window.muChatNew() starts a fresh session.
// AgentReady reports whether a model is configured, filled in by the server.
//
// A hook because internal/ai imports this package — the check lives there and
// cannot be imported back. One boolean is not a reason to invert that, and
// duplicating provider resolution here is how the two answers drift apart.
//
// Nil means yes: every test that renders this component predates the question,
// and a component that silently turned itself off when nobody had wired the
// hook would be a worse bug than the one it fixes.
var AgentReady func() bool

// searchBox is the box on an instance with no model.
//
// A GET form at /archive, not a second search: that page is already "everything
// this instance has collected… one search across all of it", with its filters
// and its result rendering. A box here that queried the index itself would be a
// second implementation of a page that exists, and the two would drift.
//
// No JavaScript either. A form that navigates is what a search box was before
// anybody had a reason to make it otherwise, and it works on the first paint.
// searchBox is the box. why is the reason it is not a chat, and is empty
// almost always.
//
// It carried that reason unconditionally, from when this rendered only on an
// instance with no model — so when search became the default everywhere, every
// instance began telling its owner it had no provider, including the ones that
// do. A note written for one case, reused for all of them, asserting something
// false on most.
//
// A search box does not need a paragraph under it explaining that it is a
// search box. The only page that owes an explanation is one that was supposed
// to be a chat.
// SearchBoxOpts is how a page asks for the box. The zero value is the plain
// search box every page had before there was a second thing to do with it.
type SearchBoxOpts struct {
	// Why this is a search box and not a chat. Empty almost always — see above.
	Why string
	// Centred is the front page, which has nothing else on it to line up with.
	// Everywhere else the box sits in a left-aligned column.
	Centred bool
	// Focused puts the cursor in it on load. True on a page whose whole purpose
	// is the box; false where it is one thing among several.
	Focused bool
}

// SearchBox renders the box. Exported because the front page draws it too, and
// it drew its own until this had two buttons — at which point there were two
// implementations of one control, and the second button would have had to be
// written twice to appear on both.
func SearchBox(o SearchBoxOpts) string { return searchBox(o) }

// askAction reports whether "Ask agent" can be offered: a model is configured.
//
// Not whether anybody is signed in. /agent refuses without an account, but a
// signed-out visitor pressing it is redirected to /login with the whole URL —
// query included — so they sign in and the question they typed is asked on
// arrival. See RedirectToLogin. The button keeps its promise; it just takes a
// step on the way, which is the truth about an agent that costs money to run.
func askAction() bool { return AgentReady == nil || AgentReady() }

// AgentIsReady is askAction for pages outside this package that draw the box
// themselves. Exported rather than exporting AgentReady, which is a hook the
// server fills in and nothing else should be reading directly.
func AgentIsReady() bool { return askAction() }

func searchBox(o SearchBoxOpts) string {
	note := ""
	if o.Why != "" {
		note = `<p class="mu-search-why">` + o.Why +
			TextLink("/admin/config", "/admin/config") + `.</p>`
	}

	// One arrow, one destination.
	//
	// An "Ask agent" button lived here beside Search for an afternoon and was
	// the wrong shape twice over. It navigated to /agent to answer, when asking
	// had always answered in place and the whole appeal of a box on the front
	// page is that it replies without taking you anywhere. And it was drawn out
	// of classes invented in this file rather than the .btn the rest of the
	// product uses, so it did not look like anything else on the screen.
	//
	// Both now belong to the chat, which is where asking already worked: where
	// there is a model, ChatComponent draws the box and offers Search beside
	// Ask, one input, the answer underneath. This is what remains where there
	// is no model — a search box, which is all it ever was.
	placeholder := "Search everything here"

	wrap := "mu-search"
	if o.Centred {
		wrap += " mu-search-mid"
	}
	focus := ""
	if o.Focused {
		focus = " autofocus"
	}

	// Its own ids, not the chat's. The two are never on a page together, so
	// sharing them would work — and would mean every selector, test and future
	// stylesheet rule naming mu-chat-form had two different forms in mind.
	return `<div id="mu-search" class="` + wrap + `"><form id="mu-search-form" method="GET" action="/archive">
    <input id="mu-search-input" type="search" name="q" placeholder="` + htmlpkg.EscapeString(placeholder) + `" maxlength="256"` + focus + `>
    <button type="submit" aria-label="Search">&#x2192;</button>
  </form>` + note + `</div>
<style>
/* Left, not centred. The rest of the page it sits on starts at the
   left margin, and a box centred inside a left-aligned column reads as
   misaligned rather than as centred. The front page is the exception: it
   passes Centred, because there is nothing on it to line up with. */
#mu-search{max-width:760px;margin:0;width:100%}
#mu-search.mu-search-mid{max-width:560px;margin:0 auto}
#mu-search-form{display:flex;align-items:center;gap:0;border:1px solid var(--card-border,#ddd);
  border-radius:6px;background:var(--card-background,#fff);padding:4px 4px 4px 12px;transition:border-color .2s}
#mu-search-form:focus-within{border-color:#999}
#mu-search-input{flex:1;border:0;outline:0;font:inherit;font-size:16px;padding:8px 0;background:transparent;
  color:var(--text-primary,#111);min-width:0}
#mu-search-form button{flex:none;border:0;border-radius:4px;background:var(--btn-primary,#111);color:#fff;
  font:inherit;width:32px;height:32px;cursor:pointer}
#mu-search-form button:hover{background:var(--btn-primary-hover,#333)}
.mu-search-why{max-width:760px;margin:8px 0 0;color:var(--text-muted,#888);font-size:13px;line-height:1.6}
</style>`
}

func ChatComponent(cfg ChatConfig) string {
	// The cards go with the question, always.
	//
	// This was a checkbox — "use live context" — off by default, remembered in
	// localStorage. Asking somebody to opt in to the product working properly
	// is asking a question they have no way to answer: the cost is tokens they
	// cannot see and the benefit is an answer they have not read yet. On Home
	// the cards are on the screen, so an answer that ignores them is the wrong
	// answer, and there is nothing to decide.
	// The same sessionStorage key the rail on /agent uses, so a choice made in
	// one place holds in the other. Two pickers disagreeing about who is
	// answering would be worse than one picker.
	agentPicker := ""
	if cfg.OfferAgentPicker {
		// The word is a field label, and is set like one.
		//
		// Sentence case, it read as a phrase: "Agent Micro (default)" is a
		// noun followed by a name, so the eye takes the whole thing as one
		// caption and the control stops looking like a control. Small caps
		// with letter-spacing is the same treatment the section headings on
		// Home get, and it does the separating that the sentence case did not.
		//
		// The alternative was dropping the word — the select does say a name.
		// It says a name and nothing about what the name is for, and a bare
		// dropdown beside a text box is a control with no question attached.
		agentPicker = `<label id="mu-chat-agent" title="Which of your agents answers. Each has its own instructions and its own scope">` +
			`<span class="mu-chat-agent-label">Agent</span>` +
			`<select id="mu-chat-agent-pick"><option value="">Micro (default)</option></select></label>`
	}

	initialConv := ""
	if cfg.InitialConvHTML != "" {
		initialConv = cfg.InitialConvHTML
	}

	// What to ask, in this agent's words when there is one — and nothing at all
	// when there is not.
	//
	// The default used to be four general suggestions and a placeholder built
	// out of the first of them, so every empty box on the site read "Try: give
	// me a morning brief". It was on the landing, on Home, on the default
	// agent, and it aged: a suggestion nobody has changed in months reads as a
	// demo rather than an offer, and the same one everywhere reads as the
	// product having one idea.
	//
	// A page that has something specific to suggest still passes it. The
	// general case gets a box that says what it is.
	suggestions := cfg.Suggestions
	placeholder := strings.TrimSpace(cfg.Placeholder)
	if placeholder == "" {
		placeholder = "Ask it something"
	}

	// Search, unless the caller has said this page is for asking.
	//
	// # Why the front page searches
	//
	// It asked, and the signed-out page searched, so the same box did two
	// different things depending on whether you were logged in — and once you
	// were, there was no consistent way to search at all.
	//
	// Which one wins is not a toss-up. google.com is a search box and a grid of
	// apps; the box is the front door and everything else is reached from
	// beside it. That is the shape this already has — a search box and
	// Services — and the agent is one of those services rather than the spine.
	// It will do more as models get faster and cheaper, and the box is where
	// that arrives; it is not what the front page is for today.
	//
	// Search is also the half that needs nothing: no model, no key, no account.
	// A page whose main control only works once you have signed up with a
	// vendor is a page that does not work.
	//
	// Asking still has a home — /agents and the agent pages pass Ask, because
	// a page about an agent is a page for talking to it.
	if !cfg.Ask {
		return searchBox(SearchBoxOpts{})
	}
	if AgentReady != nil && !AgentReady() {
		// A page that wanted a chat and cannot have one says why. Everywhere
		// else the search box is simply the box, and explains nothing. No Ask
		// button here either: this is the branch where there is nothing to ask.
		return searchBox(SearchBoxOpts{Why: "No model is configured, so the agent cannot answer yet. " +
			"Add a provider at "})
	}
	// A list with nothing in it, never null.
	//
	// json.Marshal of a nil slice is the literal null, and SUGGEST.forEach on
	// null throws — which does not fail quietly. Everything after
	// showSuggestions() in that one script block stops running: the agent
	// picker, the poll for an answer that landed while the page was gone,
	// window.muChatAsk, and the voice controls. An agent with no examples of
	// its own is the ordinary case for one somebody made, so this was the
	// whole page's JavaScript resting on a field being filled in.
	suggestJS, err := json.Marshal(suggestions)
	if err != nil || suggestions == nil {
		suggestJS = []byte("[]")
	}

	// micIcon is a microphone, drawn rather than fetched.
	//
	// Inline SVG for the reason iconMail gives: it is three lines of markup and the
	// alternative is a request for an image that has to exist, be cached and be
	// found again by whoever moves it. currentColor so it takes the button's colour
	// rather than needing one of its own.
	const micIcon = `<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">` +
		`<rect x="6" y="2" width="4" height="7" rx="2" fill="none" stroke="currentColor" stroke-width="1.3"/>` +
		`<path d="M4 7.5a4 4 0 0 0 8 0M8 11.5V14" fill="none" stroke="currentColor" ` +
		`stroke-width="1.3" stroke-linecap="round"/></svg>`

	form := `<form id="mu-chat-form">
    <textarea id="mu-chat-input" placeholder="` + htmlpkg.EscapeString(placeholder) + `" maxlength="1024" rows="1"
      onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();document.getElementById('mu-chat-form').dispatchEvent(new Event('submit'))}"
      oninput="this.style.height='auto';this.style.height=Math.min(this.scrollHeight,140)+'px'"></textarea>
    <button type="button" id="mu-chat-mic" aria-label="Speak" hidden
      title="Speak instead of typing">` + micIcon + `</button>
    <button type="submit" aria-label="Send">&#x2192;</button>
  </form>`
	// Read it back, beside who is answering.
	//
	// Next to the picker because they are the same kind of decision — how this
	// conversation happens — and because that row is where somebody already
	// looks before asking. Hidden until the script finds a voice: a control that
	// does nothing is worse than one that is not there.
	//
	// And only where an answer can arrive. It shipped on every surface with a
	// box, which includes the signed-out landing, where asking returns 401 and
	// renders a refusal — a toggle for the delivery of something that cannot be
	// delivered, on the first screen a stranger sees. See ChatConfig.Speak.
	sayToggle := ""
	if cfg.Speak {
		sayToggle = `<label id="mu-chat-say" hidden title="Read the answer out loud">` +
			`<input type="checkbox" id="mu-chat-say-on">` +
			`<span class="mu-chat-agent-label">Speak</span></label>`
	}
	opts := `<div id="mu-chat-opts">` + agentPicker + sayToggle + `</div>`
	suggest := `<div id="mu-chat-suggest"></div>`
	conv := `<div id="mu-chat-conv">` + initialConv + `</div>`

	// Two orders, one component. See ChatConfig.Transcript.
	body := form + opts + suggest + conv
	shell := `<div id="mu-chat">`
	if cfg.Transcript {
		body = conv + suggest + form + opts
		shell = `<div id="mu-chat" class="mu-chat-transcript">`
	}

	html := shell + `
  ` + body + `
</div>

<style>
#mu-chat{max-width:760px;margin:0 auto;width:100%}
#mu-chat-form{display:flex;align-items:center;gap:0;border:1px solid #ddd;border-radius:6px;background:#fff;padding:4px 4px 4px 12px;transition:border-color .2s;position:sticky;top:8px;z-index:5}
#mu-chat-form:focus-within{border-color:#999}
/* The microphone, in the box beside Send. Quiet until it is listening, then it
   is the loudest thing on the form — you need to be able to tell at a glance
   that something is recording you.

   Written as #mu-chat-form #mu-chat-mic and not as the id alone, because the
   "#mu-chat-form button" rule below is 0-1-1 and paints every button in the
   box black — so the id alone lost, and the microphone rendered as a second
   Send button sitting next to Send. */
#mu-chat-form #mu-chat-mic{width:auto;min-width:0;height:auto;background:none;border:0;border-radius:0;padding:6px 8px;cursor:pointer;color:#888;display:flex;align-items:center}
#mu-chat-form #mu-chat-mic:hover{background:none;color:#111}
#mu-chat-form #mu-chat-mic.on{color:#c00}
#mu-chat-form #mu-chat-mic.on svg{animation:mu-mic-pulse 1.2s ease-in-out infinite}
@keyframes mu-mic-pulse{0%,100%{opacity:1}50%{opacity:.35}}
/* Reading back, beside who is answering — the same kind of decision. */
#mu-chat-say{display:flex;align-items:center;gap:6px;cursor:pointer}
#mu-chat-say input{margin:0}
#mu-chat-opts{display:flex;align-items:center;gap:14px;flex-wrap:wrap;margin:6px 0 0}
#mu-chat-opts:empty{margin:0}
#mu-chat-agent{display:flex;align-items:center;gap:8px;font-size:12px;color:#999;cursor:pointer;user-select:none}
.mu-chat-agent-label{font-size:10px;text-transform:uppercase;letter-spacing:.1em;font-weight:600;color:#aaa}
#mu-chat-agent select{width:auto;padding:2px 4px;font-size:12px;font-family:inherit;color:#555;border:1px solid #e0e0e0;border-radius:4px;background:#fff}
#mu-chat-input{flex:1;padding:6px 0;border:none;font-size:16px;font-family:inherit;resize:none;line-height:1.4;overflow:hidden;background:transparent;outline:none}
/* A square icon button, sized from the control scale rather than from two
   hard-coded 36s that made it taller than every other control in the app. */
/* Fallbacks, because this component renders on a page that has no mu.css.
   The front page's shell is its own — no stylesheet link — so --control-h
   resolved to nothing there and the button collapsed to the size of the arrow
   glyph inside it: 16px, in a 34px box. Every other var in this file carries a
   fallback and these three did not, which is why the fault appeared the moment
   the box moved to a page the token does not reach. 30px is what mu.css sets. */
#mu-chat-form button{flex-shrink:0;width:var(--control-h,30px);height:var(--control-h,30px);min-width:var(--control-h,30px);padding:0;background:#111;color:#fff;border:none;border-radius:6px;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:16px;line-height:1}
#mu-chat-suggest{margin-top:16px}
.mu-pills{display:flex;gap:8px;flex-wrap:wrap;justify-content:center}
.mu-pills a{padding:8px 14px;border:1px solid #e0e0e0;border-radius:6px;font-size:13px;color:#555;text-decoration:none;cursor:pointer}
.mu-pills a:hover{background:#f5f5f5}
#mu-chat-conv{margin-top:24px;font-size:15px;line-height:1.7;text-align:left}
#mu-chat-conv:empty{margin-top:0}
/* A conversation, not a box.

   The turns scroll in their own region and the input sits under them, which is
   what every chat does. A sticky input over a page that scrolls was the first
   attempt and it is wrong in the way stickiness always is: mid-scroll it floats
   over the message you are reading, and "scroll to the bottom" means the bottom
   of the document rather than the bottom of the conversation.

   The max-height here is a fallback for the moment before the script measures
   the real one — 100dvh so a phone's collapsing chrome is accounted for. See
   fitConv, which replaces it with the exact figure and keeps it right on
   resize, so there is no constant to be wrong on somebody's screen. */
.mu-chat-transcript{display:flex;flex-direction:column}
.mu-chat-transcript #mu-chat-form{position:static;flex:none}
.mu-chat-transcript #mu-chat-opts{margin:6px 0 0;flex:none}
/* Margin only when there is something to separate.
   An empty conversation and an empty suggestion row are two zero-height
   divs, and their bottom margins still stack: 24px of nothing above the box,
   which put the input below the + New button in the column beside it.
   Measured — the button at y=110, the input at y=134.
   :empty rather than removing the margin, because both fill up the moment
   somebody says anything and the gap is right then. */
.mu-chat-transcript #mu-chat-suggest{margin:0 0 12px;flex:none}
.mu-chat-transcript #mu-chat-suggest:empty,
.mu-chat-transcript #mu-chat-conv:empty{margin:0}
/* The transcript takes the viewport less the chrome around it, and the tab bar
   at the bottom of a phone is part of that chrome. Without the subtraction the
   column is exactly the bar's height too tall and the input beneath it lands
   underneath the bar — which is what happened the day the bar was added, on
   every conversation long enough to fill the column.

   var(--tabbar, 0px) rather than a second number: mu.css sets it to 0 where
   the rail is on screen and to the bar's own height where it is not. The
   fallback is for the pages this component renders on that have no mu.css —
   they have no tab bar either, so 0 is right there too. */
.mu-chat-transcript #mu-chat-conv{
  flex:1 1 auto;min-height:0;max-height:calc(100dvh - 260px - var(--tabbar, 0px));
  overflow-y:auto;overscroll-behavior:contain;
  margin:0 0 12px;padding-right:4px
}
.mu-user{margin:0 0 12px;padding:10px 14px;background:#f5f5f5;border-radius:8px;font-size:14px;color:#333;scroll-margin-top:64px}
.mu-agent{margin-bottom:24px;scroll-margin-top:64px}
.mu-think{color:#888;font-size:14px}
.mu-err{color:#c00}
.mu-cta{padding:12px 14px;border:1px solid #e0e0e0;border-radius:8px;background:#fafafa;font-size:14px}
.mu-cta a{color:#111;font-weight:600;text-decoration:none}
.mu-cursor{display:inline-block;width:2px;height:1em;background:#000;vertical-align:text-bottom;animation:mublink .8s step-end infinite;margin-left:2px}
@keyframes mublink{0%,100%{opacity:1}50%{opacity:0}}
/* Answer content must never widen the chat column. Without this a long code
   block, an unbroken URL or a wide table stretches the flex item, which drags
   the input off-screen on mobile — the column ends up wider than the viewport.
   Each kind of wide content is contained in its own block instead. */
#mu-chat,#mu-chat-conv,#mu-chat-form{min-width:0;max-width:100%}
/* The answer wraps too.
   
   This named the transcript and the person's own message and left out the
   agent's — which is the half that contains long URLs, table rows and fenced
   code, so it is the only half that could push the page wide. A pre does not
   wrap at all, so it gets a scroller of its own rather than making one for
   the whole document. */
#mu-chat-conv,.mu-user,.mu-agent{overflow-wrap:break-word;min-width:0}
.mu-agent pre,.mu-agent table{max-width:100%;overflow-x:auto}
.mu-agent img{max-width:100%;height:auto}
#mu-chat-conv pre{overflow-x:auto;max-width:100%}
#mu-chat-conv table{display:block;overflow-x:auto;max-width:100%}
#mu-chat-conv img{max-width:100%;height:auto}
#mu-chat .card{max-width:100%;border:1px solid #e0e0e0;border-radius:8px;padding:16px 18px;margin-bottom:12px;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.04)}
#mu-chat .card h4{margin:0 0 8px;font-size:1em;font-weight:600}
#mu-chat .card a,#mu-chat .link,#mu-chat a.link{color:#111}
/* Self-contained typography so rendered answers look right anywhere. */
#mu-chat .card h1,#mu-chat .card h2,#mu-chat .card h3{margin:16px 0 8px;line-height:1.3;font-weight:700}
#mu-chat .card h1{font-size:1.3em}
#mu-chat .card h2{font-size:1.15em}
#mu-chat .card h3{font-size:1.02em}
#mu-chat .card>h1:first-child,#mu-chat .card>h2:first-child,#mu-chat .card>h3:first-child,#mu-chat .card>p:first-child{margin-top:0}
#mu-chat .card p{margin:0 0 10px}
#mu-chat .card ul,#mu-chat .card ol{margin:0 0 12px;padding-left:22px}
#mu-chat .card li{margin:0 0 5px}
#mu-chat .card hr{border:none;border-top:1px solid #eee;margin:14px 0}
#mu-chat .card>*:last-child{margin-bottom:0}
.mu-think{display:flex;align-items:center}
.mu-spin{display:inline-block;width:11px;height:11px;border:2px solid #ddd;border-top-color:#666;border-radius:50%;animation:muspin .7s linear infinite;margin-right:8px;flex-shrink:0}
@keyframes muspin{to{transform:rotate(360deg)}}
.mu-think-t{color:#bbb;font-size:12px;margin-left:6px}
</style>

<script>
(function(){
var contextId=` + JSString(cfg.ContextID) + `;
var SESSION=` + boolJS(cfg.InitialConvHTML != "") + `;
var PENDING=` + boolJS(cfg.Pending) + `;
var HIDE_SUGGEST=` + boolJS(cfg.HideSuggestions) + `;
var form=document.getElementById('mu-chat-form');
var input=document.getElementById('mu-chat-input');
var conv=document.getElementById('mu-chat-conv');
// A transcript keeps its input at the bottom and its newest turn above it.
// See ChatConfig.Transcript.
var transcript=!!document.querySelector('#mu-chat.mu-chat-transcript');

// The brief steps aside when you ask.
//
// The brief and the answer are the two halves of the same idea — one is what
// you are told without asking, the other is what you get when you do — and they
// are both a paragraph of prose. Stacked, the answer arrives under a sentence
// about the day and the reader has to work out where one stops. Worse, the
// brief is pushed down the page by every turn, so the thing that was the point
// of the page ends up below a conversation.
//
// So asking replaces it, and starting a fresh session brings it back. Anything
// marked data-brief takes part; the landing page and Home both mark theirs, and
// nothing else has to know this exists.
function briefs(){return document.querySelectorAll('[data-brief]');}
function hideBrief(){var n=briefs();for(var i=0;i<n.length;i++){n[i].hidden=true;}}
function showBrief(){var n=briefs();for(var i=0;i<n.length;i++){n[i].hidden=false;}}
// nearBottom is the difference between "following the answer" and "reading
// something further up". Scrolling to the bottom in the second case is the
// thing that makes a chat unusable while a long answer streams.
function nearBottom(){
  if(!conv) return true;
  return (conv.scrollTop+conv.clientHeight)>=(conv.scrollHeight-nearEnough);
}
var nearEnough=120;
function toBottom(force,smooth){
  if(!transcript) return;
  if(!force && !nearBottom()) return;
  requestAnimationFrame(function(){
    conv.scrollTo({top:conv.scrollHeight,behavior:smooth?'smooth':'auto'});
  });
}
// The conversation is as tall as what is left of the window under it.
//
// Measured rather than guessed: the page above this has a header, a chip and
// whatever the host put there, and every constant anybody writes for that is
// wrong on some screen. getBoundingClientRect knows where the region starts and
// the form knows how tall it is, so the arithmetic is exact and it is redone
// whenever the window changes — including when the textarea grows as somebody
// types a long message.
function fitConv(){
  if(!transcript||!conv) return;
  var top=conv.getBoundingClientRect().top;
  var below=0;
  var form=document.getElementById('mu-chat-form');
  var opts=document.getElementById('mu-chat-opts');
  var sug=document.getElementById('mu-chat-suggest');
  [form,opts,sug].forEach(function(el){ if(el) below+=el.offsetHeight; });
  var h=Math.max(minConv, window.innerHeight-top-below-convGap);
  conv.style.maxHeight=h+'px';
}
// minConv keeps the region usable on a short window rather than collapsing it
// to nothing; convGap is the breathing room under the input.
var minConv=220, convGap=28;
var sugDiv=document.getElementById('mu-chat-suggest');
if(!form)return;
var NS=` + JSString(cfg.StorageNS) + `;
// Persistence is per-surface and opt-in. With no namespace the component is
// ephemeral (Home's quick-ask box) so it never restores or leaks a
// conversation — in particular it must not show the /agent app's thread.
var PERSIST=!!NS;
var CKEY='mu_chat_conv:'+NS;
var HKEY='mu_chat_hist:'+NS;
var TKEY='mu_chat_ctx:'+NS;
var history=[];

// A reopened server session is authoritative; otherwise restore this surface's
// own in-tab conversation so a reload doesn't lose it — including the server
// thread id (context_id), so a follow-up after a reload continues the same
// conversation instead of starting a new one.
if(!SESSION && PERSIST){
  try{
    var savedConv=sessionStorage.getItem(CKEY);
    if(savedConv)conv.innerHTML=savedConv;
    var savedHist=sessionStorage.getItem(HKEY);
    if(savedHist)history=JSON.parse(savedHist)||[];
    var savedCtx=sessionStorage.getItem(TKEY);
    if(savedCtx)contextId=savedCtx;
  }catch(e){}
}

// A conversation already on the page means the asking has happened, whether it
// came back from sessionStorage or was rendered by the server. Without this a
// reload put the brief back above a transcript that was still there.
if(conv.innerHTML.trim())hideBrief();

function esc(s){return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}
function protectCurrencyDollars(s){return String(s||'').replace(/\$(?=\d)/g,'$\u2060');}

var SUGGEST=` + string(suggestJS) + `;
function showSuggestions(){
  if(HIDE_SUGGEST){sugDiv.innerHTML='';return;}
  if(conv.innerHTML.trim()){sugDiv.innerHTML='';return;}
  // No guest note here. The page that renders a guest chat says it once, in
  // its own markup, under the box — and this said it a second time directly
  // above that, so the landing page told a visitor the same thing twice in two
  // stacked paragraphs. One offer, stated once, by whoever owns the page.
  var h='<div class="mu-pills">';
  SUGGEST.forEach(function(s){h+='<a href="#" data-q="'+esc(s)+'">'+esc(s)+'</a>';});
  h+='</div>';
  sugDiv.innerHTML=h;
  sugDiv.querySelectorAll('a').forEach(function(a){
    a.addEventListener('click',function(e){e.preventDefault();input.value=a.dataset.q;ask(a.dataset.q);});
  });
}

function save(){
  if(SESSION||!PERSIST)return; // server owns reopened sessions; ephemeral surfaces don't save
  try{
    sessionStorage.setItem(CKEY,conv.innerHTML);
    sessionStorage.setItem(HKEY,JSON.stringify(history.slice(-6)));
    sessionStorage.setItem(TKEY,contextId||'');
  }catch(e){}
}

function ask(q){
  q=String(q||'').trim();
  if(!q)return;
  hideBrief();
  sugDiv.innerHTML='';
  var u=document.createElement('div');u.className='mu-user';u.textContent=q;conv.appendChild(u);
  var a=document.createElement('div');a.className='mu-agent';conv.appendChild(a);
  input.value='';input.style.height='auto';input.focus();

  var workLabel='Working';
  var t0=Date.now();
  var timer=null;
  function renderWork(){
    var dots=['.','..','...'][Math.floor((Date.now()-t0)/450)%3];
    var secs=Math.round((Date.now()-t0)/1000);
    a.innerHTML='<div class="mu-think"><span class="mu-spin"></span><span>'+esc(workLabel)+dots+'</span>'+(secs>=1?'<span class="mu-think-t">'+secs+'s</span>':'')+'</div>';
  }
  function startWork(label){if(label)workLabel=label;renderWork();if(!timer)timer=setInterval(renderWork,450);}
  // How many tools are in flight, so the label goes back to the plain one only
  // when they have all finished.
  var running=0;
  function stopWork(){if(timer){clearInterval(timer);timer=null;}}
  startWork('Working');

  save();
  // Where to look after asking.
  //
  // In a transcript the newest turn is at the bottom, so that is where the eye
  // goes — and it keeps going there while the answer streams, but only while
  // the reader has not scrolled away. Chasing the bottom when somebody has
  // deliberately scrolled up to read something is the behaviour every chat gets
  // wrong once.
  //
  // In a box the exchange is anchored under the input instead, which is what
  // Home wants: you typed at the top and the answer appears under it.
  if(transcript){ toBottom(true,true); } else { u.scrollIntoView({behavior:'smooth',block:'start'}); }
  var streamText='';
  var streaming=false;
  var body=JSON.stringify({prompt:q,history:history.slice(-6),context_id:contextId||'',agent:(window.muActiveAgent||''),cards:true});
  fetch('/agent',{method:'POST',headers:{'Content-Type':'application/json'},body:body,credentials:'same-origin'})
  .then(function(resp){
    if(resp.status===401){
      return resp.json().catch(function(){return {};}).then(function(j){
        stopWork();
        // A refusal has to leave somebody able to do something.
        //
        // It said "Sign in to ask the agent" with two links, on the front page,
        // which is the page a stranger sees and the only page this branch ever
        // renders on. So the main control of the signed-out product answered
        // every question with a sign-up form — worse than the search box it
        // replaced, which at least worked.
        //
        // Two ways on, and the first one works this second. The archive is
        // public by construction, so searching what was just typed needs no
        // account and no permission; signing in carries the question through
        // and asks it on arrival, which is the same trip /agent?q= already
        // makes. Neither is a form standing between somebody and an answer.
        var msg=esc(j.error||'Nobody can ask the agent here without an account.');
        var qq=encodeURIComponent(q);
        a.innerHTML='<div class="mu-cta">'+msg+
          ' <a href="/archive?q='+qq+'">Search the archive for this →</a>'+
          ' <a href="/login?redirect='+encodeURIComponent('/agent?q='+q)+'" class="ml-3">Sign in and ask it</a></div>';
        save();
        throw 'handled';
      });
    }
    if(!resp.ok||!resp.body){stopWork();a.innerHTML='<div class="mu-err">Something went wrong. Please try again.</div>';save();throw 'handled';}
    var reader=resp.body.getReader();
    var decoder=new TextDecoder();
    var buf='';
    function read(){
      return reader.read().then(function(chunk){
        if(chunk.done){stopWork();save();return;}
        buf+=decoder.decode(chunk.value,{stream:true});
        var lines=buf.split('\n');
        buf=lines.pop();
        lines.forEach(function(line){
          if(line.indexOf('data: ')!==0)return;
          try{
            var ev=JSON.parse(line.slice(6));
            if(ev.type==='flow_id'){
              // Continue this server session on the next message. Persist it now
              // (it arrives before the answer) so a reload mid-stream still
              // threads the follow-up onto this same conversation.
              // A conversation that did not exist a moment ago is one the page
              // beside this can now list. Without this the rail only learned
              // about a conversation on the next reload, so saying hello and
              // then refreshing made a row appear that had not been there while
              // you were talking — the list looked like it was lagging reality,
              // because it was. Optional: the same component runs on Home and
              // the landing page, where there is no rail to tell.
              // The conversation, where there is one — a signed-in caller. A
              // guest is not recorded, so the run id carries the thread for the
              // length of the page and no further.
              var id=ev.thread||ev.flow_id;
              if(id){
                var fresh=!contextId;
                contextId=id;save();
                if(fresh&&window.muSessionStarted)window.muSessionStarted(id,q);
              }
            }else if(ev.type==='thinking'){
              startWork(ev.message);
            }else if(ev.type==='tool_start'){
              // What it is doing, while it does it. The server has been
              // sending these all along and nothing here listened, so a run
              // that searched the web and read your mail said "Processing" for
              // the whole minute and then produced an answer out of nowhere.
              running++;
              startWork(ev.message||'Working');
            }else if(ev.type==='tool_done'){
              // Back to the plain label only when the last one finishes — tools
              // can run together, and one of three ending does not mean the
              // work has stopped.
              //
              // "Thinking" is what this said, and it is two wrong things at
              // once: the model is not thinking, and between two tool calls it
              // is picking the next command, which the very next tool_start
              // will name anyway. Working is true and brief.
              if(running>0)running--;
              if(running===0)startWork('Working');
            }else if(ev.type==='stream_start'){
              streamText='';streaming=false;startWork('Composing');
            }else if(ev.type==='stream_token'){
              streamText+=ev.token;
              if(!streaming){
                streaming=true;stopWork();
                a.innerHTML='<div class="whitespace-pre-wrap"><span id="mu-stream-out"></span><span class="mu-cursor"></span></div>';
              }
              var el=document.getElementById('mu-stream-out');
              if(el)el.textContent=protectCurrencyDollars(streamText);
              // Follow the answer down, unless the reader has scrolled away to
              // look at something else.
              toBottom(false);
            }else if(ev.type==='response'){
              stopWork();
              a.innerHTML=ev.html;
              history.push({prompt:q,answer:streamText});
              save();
              toBottom(false);
              // Out loud, when that was asked for. streamText and not ev.html:
              // a voice reading markup says "less than div" at you.
              if(window.muSay)window.muSay(streamText);
            }else if(ev.type==='error'){
              stopWork();
              a.innerHTML='<div class="mu-err">'+esc(ev.message)+'</div>';
              save();
            }
          }catch(ex){}
        });
        return read();
      });
    }
    return read();
  })
  .catch(function(err){
    stopWork();
    if(err==='handled')return;
    a.innerHTML='<div class="mu-err">Error: '+esc(err&&err.message||err)+'</div>';
    save();
  });
}

form.addEventListener('submit',function(e){e.preventDefault();ask(input.value);});
showSuggestions();

// Start a fresh session (clears the log + thread id).
window.muChatNew=function(){
  conv.innerHTML='';history=[];contextId='';
  try{sessionStorage.removeItem(CKEY);sessionStorage.removeItem(HKEY);sessionStorage.removeItem(TKEY);}catch(e){}
  showBrief();
  showSuggestions();input.focus();
};
// Exposed so server-rendered prefill (?q= / ?prompt=) can auto-submit.

// Which agent answers. The component has always sent window.muActiveAgent and
// the server has always honoured it; until now the only thing that could set it
// was the rail on /agent, so an agent created on /agents could not be reached
// from the input on Home.
//
// Same sessionStorage key as that rail, so the choice holds across both.
(function(){
  var sel=document.getElementById('mu-chat-agent-pick');
  if(!sel) return;
  var KEY='mu_active_agent';
  try{ window.muActiveAgent=window.muActiveAgent||sessionStorage.getItem(KEY)||''; }catch(e){}

  // Where to write to whichever agent is answering.
  //
  // The host page renders the address; the id is the contract between them.
  // Picking an agent changed which name answered and nothing else on the
  // screen, so the page read "answering as Test" directly above "write to it
  // at agent@" — the first question anybody has about an agent, answered
  // wrongly by the one control that should have changed it.
  var addrs={};
  function showAddr(){
    var el=document.getElementById('mu-agent-addr');
    if(!el) return;
    var a=addrs[sel.value]||addrs[''];
    if(a) el.textContent=a;
  }

  sel.addEventListener('change',function(){
    window.muActiveAgent=sel.value;
    try{ sessionStorage.setItem(KEY, sel.value); }catch(e){}
    showAddr();
  });

  fetch('/agents/data',{headers:{'Accept':'application/json'}})
    .then(function(r){return r.json();})
    .then(function(d){
      // Every agent. There used to be a filter here for ones declared
      // "external" — a credential and a scope for something calling in from
      // outside, which nothing here could hand a question to, so "answering as"
      // it meant Micro answering with that agent's scope and an empty prompt.
      // That kind is gone: every agent runs here and has a prompt.
      var list=(d&&d.agents)||[];
      // No agents, nothing to choose between: a picker with one option is a
      // control that only takes up room.
      if(!list.length){ var l=document.getElementById('mu-chat-agent'); if(l) l.remove(); return; }
      // Rebuilt, not appended to. This ran twice on one page — a soft
      // navigation back to Home re-runs it against a select that already has
      // its options — and every agent appeared once per run, so the list read
      // Micro, Test, Test. Building the whole list is right however many times
      // it happens.
      sel.innerHTML='';
      var def=document.createElement('option');
      def.value=''; def.textContent='Micro (default)';
      sel.appendChild(def);
      addrs={'':(d&&d.address)||''};
      list.forEach(function(a){
        var o=document.createElement('option');
        o.value=a.id; o.textContent=a.name;
        if(a.description) o.title=a.description;
        sel.appendChild(o);
        if(a.address) addrs[a.id]=a.address;
      });
      // Restore the choice, unless that agent has since been removed.
      if(window.muActiveAgent){
        sel.value=window.muActiveAgent;
        if(sel.value!==window.muActiveAgent){ window.muActiveAgent=''; sel.value=''; }
      }
      showAddr();
    })
    .catch(function(){ var l=document.getElementById('mu-chat-agent'); if(l) l.remove(); });
})();
window.muChatAsk=ask;

// Speaking, and being spoken to.
//
// Both are the browser's, both are optional, and each is drawn only where it
// works — a microphone button that does nothing when pressed is worse than no
// microphone button, and Firefox has no SpeechRecognition at all.
//
// # The microphone fills the box, it does not send
//
// Dictation is wrong often enough that sending on the last word would mean
// watching your own mistake go out. It types for you and stops; the send is
// still yours, and the box is still editable, which is the difference between a
// faster input and a worse one.
//
// Interim results are shown as they arrive. Without them the button looks
// broken for the length of a sentence, which is exactly when somebody presses
// it again and stops the thing that was working.
(function(){
  var box=document.getElementById('mu-chat-input');
  var mic=document.getElementById('mu-chat-mic');
  var SR=window.SpeechRecognition||window.webkitSpeechRecognition;
  if(box&&mic&&SR){
    mic.hidden=false;
    var rec=null,on=false,settled='';
    mic.addEventListener('click',function(){
      if(on&&rec){rec.stop();return;}
      rec=new SR();
      rec.lang=document.documentElement.lang||'en-GB';
      rec.interimResults=true;
      rec.continuous=false;
      // What was already in the box stays. Somebody who typed half a sentence
      // and then reached for the microphone meant to finish it, not replace it.
      settled=box.value?box.value.replace(/\s*$/,'')+' ':'';
      rec.onstart=function(){on=true;mic.classList.add('on');mic.setAttribute('aria-label','Stop');};
      rec.onresult=function(e){
        var said='';
        for(var i=e.resultIndex;i<e.results.length;i++)said+=e.results[i][0].transcript;
        box.value=settled+said;
        box.style.height='auto';box.style.height=Math.min(box.scrollHeight,140)+'px';
      };
      var off=function(){on=false;mic.classList.remove('on');mic.setAttribute('aria-label','Speak');rec=null;box.focus();};
      rec.onerror=off;rec.onend=off;
      try{rec.start();}catch(e){off();}
    });
  }

  // And back. The toggle is remembered per browser, which is the right scope
  // for it: whether the room you are sitting in is one you can have a machine
  // talking out loud in is a fact about the room, not about the account.
  var say=document.getElementById('mu-chat-say');
  var sayOn=document.getElementById('mu-chat-say-on');
  if(say&&sayOn&&window.speechSynthesis){
    say.hidden=false;
    try{sayOn.checked=localStorage.getItem('mu_speak')==='1';}catch(e){}
    sayOn.addEventListener('change',function(){
      try{localStorage.setItem('mu_speak',sayOn.checked?'1':'0');}catch(e){}
      // Silence now, not after the current sentence. Turning it off is
      // something somebody does because it is talking and they want it to stop.
      if(!sayOn.checked)window.speechSynthesis.cancel();
    });
    window.muSay=function(text){
      if(!sayOn.checked)return;
      text=String(text||'').trim();
      if(!text)return;
      // One answer at a time. Asking again before the last one finished would
      // otherwise queue them and read both.
      window.speechSynthesis.cancel();
      window.speechSynthesis.speak(new SpeechSynthesisUtterance(text));
    };
  }
})();

// An answer that landed while the page was gone.
//
// PENDING is set when the server rendered this conversation and the last thing
// in it was your question. The run that is answering it does not stop when the
// page reloads — it finishes and writes the reply into the record — so the only
// thing missing is somebody looking. This looks.
//
// The spinner first, because a refresh mid-run left the question sitting there
// with nothing under it and no way to tell "still working" from "this is
// broken". That was the whole report.
if(PENDING&&contextId&&conv){(function(){
  var a=document.createElement('div');a.className='mu-agent';conv.appendChild(a);
  var t0=Date.now(),timer=null;
  function draw(){
    var dots=['.','..','...'][Math.floor((Date.now()-t0)/450)%3];
    var secs=Math.round((Date.now()-t0)/1000);
    a.innerHTML='<div class="mu-think"><span class="mu-spin"></span><span>Working'+dots+
      '</span>'+(secs>=1?'<span class="mu-think-t">'+secs+'s</span>':'')+'</div>';
  }
  draw();timer=setInterval(draw,450);
  function done(html){
    if(timer){clearInterval(timer);timer=null;}
    if(html){a.outerHTML=html;}else{a.remove();}
    toBottom(false);
  }

  // Every three seconds, and it gives up.
  //
  // A run whose process died — a deploy, a crash — leaves a question with no
  // answer coming, and there is nothing in the record that tells those apart
  // from a slow one: both are "the newest message is yours". Time is the only
  // thing that separates them, so after ten minutes this says so rather than
  // spinning for the life of the tab. Ten because a build with a dozen tool
  // calls in it is minutes, and the failure this replaces was silence.
  var every=3000,giveUp=Date.now()+600000;
  function poll(){
    fetch('/agent/pending?thread='+encodeURIComponent(contextId),
      {headers:{'Accept':'application/json'},credentials:'same-origin'})
      .then(function(r){return r.ok?r.json():null})
      .then(function(d){
        if(!d){done('');return;}
        if(d.html){done(d.html);return;}
        if(!d.waiting){done('');return;}
        if(Date.now()>giveUp){
          done('<div class="mu-agent"><div class="card">No answer came back. '+
            'The run may have stopped when the server restarted — ask again.</div></div>');
          return;
        }
        setTimeout(poll,every);
      })
      .catch(function(){setTimeout(poll,every);});
  }
  setTimeout(poll,every);
})();}

// A reopened conversation opens at its end, which is where the reading is.
// Instant rather than smooth: this is the position the page should have loaded
// in, not a movement to watch.
//
// Once was not enough. Scrolling to the bottom asks for scrollHeight, and at
// that moment the page has not finished becoming its size — a webfont swaps, a
// card renders, an image arrives — so it went to the bottom of a shorter page
// and stopped there, leaving somebody to scroll down to the message they had
// just reloaded to see.
//
// So it stays pinned to the end until the reader scrolls away from it, which
// is also the only signal that says they want to be somewhere else. Anything
// that changes the height afterwards — late layout, a slow card — keeps it
// where it was put, and the moment they scroll up it lets go for good.
var pinned=true;
if(conv){
  conv.addEventListener('scroll',function(){ if(!nearBottom()) pinned=false; });
}
function pin(){ if(pinned) toBottom(true); }
fitConv();
toBottom(true);
window.addEventListener('load',function(){ fitConv(); pin(); });
if(conv&&window.ResizeObserver){ new ResizeObserver(pin).observe(conv); }
window.addEventListener('resize',function(){ fitConv(); toBottom(false); });
if(input) input.addEventListener('input',fitConv);
})();
</script>`

	return html
}

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
