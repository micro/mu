// Package agent provides a conversational AI agent interface that has access
// to all Mu tools via the MCP server, using the user's session token for calls.
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/agent/micro"
	"mu/inbox"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/thread"
	"mu/service/mail"
)

// There is one model tier.
//
// There were two — Fast on the default provider and Best routed to Anthropic at
// twenty credits — selected by a "model" field in the request JSON. Nothing sent
// it. The Fast/Best select that did was on a second agent form on the home
// screen, deleted a while back for being a duplicate of the prompt box beside
// it, and the tier machinery outlived the only control that set it: a slice, a
// lookup loop in two places, a second priced operation and an environment
// variable naming an Anthropic model, all to choose the one entry that was
// always chosen.
//
// If a premium tier comes back it needs a control, a price somebody has decided
// on, and something on the page saying what the difference buys. That is a
// feature to build, not a switch to leave lying around.

// QuotaCheck is set by main.go to wire in the wallet quota check without an
// import cycle. Signature matches api.QuotaCheck.
var QuotaCheck func(r *http.Request, op string) (bool, int, error)

// ChargeQuota is set by main.go to deduct credits from the acting user's wallet
// once an agent query is admitted. Charging the user who runs the agent (not any
// app owner) is deliberate. Admins and self-hosted instances are unaffected.
var ChargeQuota func(r *http.Request, op string)

// Load initialises the agent package.
func Load() {
	// Conversations the chat kept as workflow records before there was a record
	// to keep them in. The rail reads the record now, so without this somebody's
	// history would appear to have been deleted.
	adoptAll()

	// Which tools produced an answer, for the inbox to show beside it.
	//
	// The inbox reads the record — what was said — and the workflow store is the
	// agent's, because how an answer was produced is a different question with a
	// different lifetime. So the inbox asks, rather than importing: it must not
	// depend on the agent to render a conversation somebody had by mail.
	inbox.Tools = runTools
}

// runTools is what the run behind an answer actually called.
//
// This is where the workflow record surfaces, and it is the whole of what the
// Runs page was for: which tools an answer came from, and the error if it
// failed — in the conversation, next to the answer, rather than on a page of
// its own listing the same events with none of the context.
func runTools(workflow string) string {
	f := getFlow(workflow)
	if f == nil {
		return ""
	}
	var chips strings.Builder
	seen := map[string]bool{}
	for _, s := range f.Steps {
		if s.Tool == "" || seen[s.Tool] {
			continue
		}
		seen[s.Tool] = true
		chips.WriteString(app.Pill(s.Tool))
	}
	if f.Status == "error" && f.Error != "" {
		chips.WriteString(`<span class="ib-failed">` + htmlEsc(f.Error) + `</span>`)
	}
	if chips.Len() == 0 {
		return ""
	}
	return `<div class="ib-ran">` + chips.String() + `</div>`
}

// QueryMessage is a single turn in a conversation.
type QueryMessage struct {
	Role string // "user" or "assistant"
	Text string
}

// QueryOpts controls what context is included in agent queries.
type QueryOpts struct {
	History []QueryMessage
	Public  bool   // if true, skip private context (mail, wallet, etc.)
	System  string // optional custom system prompt (user-defined agent)
	// Extra is context for this call only — today, the summary of the cards
	// the reader watches, passed when they ask for it. Per-call rather than a
	// package hook because it is a choice made per message: context costs
	// tokens on every turn and most questions have nothing to do with it.
	Extra string
	Tools []string // optional tool allow-list (user-defined agent); empty = all
	// Model is which model to answer with, when this caller has a preference.
	//
	// Per-agent rather than per-instance because the jobs differ in what they
	// need: a lookup is one round and any competent model does it, while a long
	// run of tool calls rewards the model that holds together over ten of them.
	// One global setting makes that a single decision for both, which means
	// paying frontier prices for a weather question or scrimping on the one
	// that builds things.
	//
	// Empty means the instance's own choice — see nativeLLMFor. A model whose
	// provider has no key here is ignored with a line in the log, the same as a
	// mistyped AGENT_MODEL, because an agent that answers beats one that fails
	// closed over configuration.
	Model string
	// Stream reports the answer as it is produced, for a client that shows it
	// arriving. Zero value means nobody is listening, which is every client
	// but the web today — and the reason streaming was unreachable to them is
	// that it lived in the SSE handler rather than here.
	Stream StreamHooks
	// OnStep is called once per tool the agent runs, if set.
	//
	// The pipeline discarded this: a caller got a final string and no way to
	// know what produced it. That is tolerable for a chat reply, where the
	// answer arrives while you are watching, and useless for a task run, which
	// happens while you are not — "it did something and here is a paragraph"
	// is not something anyone can check.
	//
	// A callback rather than a return value or a package-level hook: it needs
	// to work for the micro path too, and two runs by the same account at the
	// same time must not interleave into one list.
	OnStep func(Step)
}

// microStepper adapts OnStep for the micro path, which cannot import this
// package.
func microStepper(opts QueryOpts) func(string, map[string]any, bool, time.Duration) {
	if opts.OnStep == nil {
		return nil
	}
	return func(tool string, args map[string]any, ok bool, took time.Duration) {
		opts.OnStep(Step{Tool: tool, Args: args, OK: ok, Took: took})
	}
}

// Step is one tool the agent ran.
type Step struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
	OK   bool           `json:"ok"`
	Took time.Duration  `json:"took"`
}

// Query runs the agent pipeline synchronously for MCP and bot callers.
func Query(accountID, prompt string, history ...QueryMessage) (string, error) {
	return QueryWithOpts(accountID, prompt, QueryOpts{History: history})
}

// Routed honours an agent named in the prompt, and returns the prompt and
// options to run with.
//
// Naming one *sets the options* rather than diverting into a separate pipeline.
// That is what makes the choice survive: micro.Orchestrate took no system prompt
// and built its own from the registry, so diverting there discarded whatever the
// caller had said — and it could not stream, which is why the web skipped
// routing altogether and quietly became the one client answering as the
// generalist while the other four got specialists. Same question, different
// agent, depending on how you happened to arrive. Orchestrate is deleted; this
// shape is why, and is worth keeping stated.
//
// One agent, not several. The old path could name three and ran them in
// parallel and merged the answers; a single question wants a single answerer,
// and fanning out multiplies the cost of every turn for a synthesis nobody
// asked for.
func Routed(prompt string, opts QueryOpts) (string, QueryOpts) {
	// A caller who supplied a system prompt has already chosen the agent, and
	// the router must not choose a different one.
	//
	// The same bug the deleted planner had one level down, where every task and
	// scheduled run composed as Micro whichever agent it was for.
	if strings.TrimSpace(opts.System) != "" {
		return prompt, opts
	}

	// Addressed, or nothing. The keyword router that used to guess a specialist
	// out of the words went with the specialists: it named ten agents that no
	// longer exist, so every question it "routed" landed on the fallback anyway.
	// Asking for an agent by name is the one signal that was never a guess.
	id := micro.MatchDirectAddress(prompt)
	if id == "" {
		return prompt, opts
	}
	prompt = micro.StripAddress(prompt)
	if id == DefaultPlatformAgent {
		return prompt, opts
	}
	if o := PlatformOpts(Platform(id)); o.System != "" {
		opts.System, opts.Tools = o.System, o.Tools
	}
	return prompt, opts
}

// QueryWithOpts runs the agent with explicit options.
// Routes to specialised micro-agents when possible.
func QueryWithOpts(accountID, prompt string, opts QueryOpts) (string, error) {
	// A caller who supplied a system prompt has already chosen the agent, and
	// the router must not choose a different one.
	//
	// Orchestrate takes no system prompt — Execute builds its own from the
	// registry entry — so every routed run silently dropped opts.System. Two
	// things went out of the window together: the agent somebody addressed, and
	// any instruction about the medium. Mail is where it showed. Writing to
	// agent+news@ about markets got the markets specialist, because the words
	// won over the address; and the framing that tells an agent it is answering
	// an email — do the work, do not offer to draft — was discarded for exactly
	// the questions that route, which is most of them. That is how a request
	// for news came back as a draft email addressed to itself.
	//
	// The same bug the deleted planner had one level down, where every task and
	// scheduled run composed as Micro whichever agent it was for.
	prompt, opts = Routed(prompt, opts)

	// The agent answers. There is no second implementation to fall through to.
	//
	// There was: a plan/execute/synthesize pipeline written before go-micro had
	// an agent, which asked one model for a JSON array of tool calls, ran them,
	// and asked another model to write prose around the results. It survived as
	// an error fallback, and that is the shape worth naming, because a fallback
	// that behaves differently from the thing it replaces does not make failure
	// softer — it makes it quieter. A run that errored here silently became a
	// run by a different agent, with a different tool catalogue, a different
	// system prompt, no memory of the conversation and no user-defined agent
	// applied, and returned an answer nobody could tell apart from a real one.
	// The place it hurt was the place nobody was watching: a scheduled task
	// whose model call timed out came back composed as Micro, from a
	// hand-maintained list of tools that had drifted from the registry.
	//
	// So the error is the answer now. See ErrNoProvider in native.go for the
	// same argument about an unconfigured instance.
	answer, err := runNative(accountID, prompt, opts)
	if err != nil {
		return "", err
	}
	return app.NormalizeAnswerMarkdown(answer), nil
}

// Handler dispatches GET (page) and POST (query) at /agent and /agent/*.
func Handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// /agent/flow/<id>  — view or delete a saved flow
	if strings.HasPrefix(path, "/agent/flow/") {
		id := strings.TrimPrefix(path, "/agent/flow/")
		switch r.Method {
		case "GET":
			// Saved flows are conversations now — reopen in the unified chat.
			if id != "" && r.URL.Query().Get("json") != "1" {
				// The agent page, where a conversation you started is read —
				// /inbox is what arrived and does not have this one in it.
				http.Redirect(w, r, "/agent/"+DefaultSlug+"?session="+url.QueryEscape(id),
					http.StatusFound)
				return
			}
			serveFlowPage(w, r, id) // ?json=1 still returns the flow JSON for polling
		case "DELETE":
			handleDeleteFlow(w, r, id)
		default:
			app.MethodNotAllowed(w, r)
		}
		return
	}
	switch r.Method {
	case "GET":
		servePage(w, r)
	case "POST":
		// /agent/<name> is a place: GET reads the conversation, POST asks a
		// question.
		if strings.TrimPrefix(path, "/agent/") != "" && path != "/agent" {
			APIHandler(w, r)
			return
		}
		// /agent is both doors, told apart by what the caller asks for.
		//
		// It was the page's streaming box alone, so the shortest address a
		// program could use was /agent/micro — which names the default agent,
		// and naming it is the one thing a caller should not have to do.
		// agent@<domain> has never needed a name; these two doors should read
		// the same way.
		//
		// Routed on Accept, not on the shape of the body. The first attempt
		// keyed on the field name — the page sent "prompt" and the API sent
		// "text" — which worked and encoded an inconsistency as a rule: two
		// names for one thing, load-bearing. Everything in this codebase sends
		// an agent a prompt, so the API takes a prompt too, and what separates
		// the callers is the honest difference between them. handleQuery
		// answers text/event-stream and nothing else; a program wants an
		// answer, not a stream of one.
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			handleQuery(w, r)
			return
		}
		APIHandler(w, r)
	default:
		app.MethodNotAllowed(w, r)
	}
}

// servePage renders the unified chat surface: a sessions rail (logged-in users)
// beside the shared chat component, in the standard app shell. Reopening a saved
// session loads its turns in place and continues the same conversation. This is
// the single AI surface — the home assistant and /agent render the same chat
// component.
//
// Signed in, or not at all. A signed-out visitor used to get this page with
// three free runs a day and a paragraph explaining what they were looking at;
// that was the landing's demonstration, and the landing does not have a chat
// box on it any more. Every account gets a daily allowance of its own, so the
// way to try this is to sign up — which is what the landing's one button does.
func servePage(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	if acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	accountID := acc.ID

	// Reopen a saved session (?session=, or legacy ?continue=).
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		sessionID = r.URL.Query().Get("continue")
	}
	// This page is for talking to an agent and requires a session to reach,
	// so an answer arrives here — which is what Speak is a control over.
	cfg := app.ChatConfig{StorageNS: "agent", Speak: true}
	activeRoot := "" // the reopened conversation, for the rail highlight
	reopened := false
	reopenAgent := "" // agent the reopened conversation is with
	// elsewhere is set when the opened conversation happened on another client.
	// It is read rather than continued: typing into a box here would send
	// nothing, because the thread it belongs to is an email chain or a WhatsApp
	// exchange and the way to carry it on is to reply there.
	elsewhere := ""
	if sessionID != "" {
		// A conversation is a conversation in the record. It used to be a chain
		// of workflow records linked by ParentID, which is why the chat and the
		// conversation list disagreed about what had been said, and why an
		// evicted run took a conversation with it. Old links still work — see
		// openThread.
		if id := openThread(accountID, sessionID); id != "" {
			th := thread.Get(accountID, id)
			activeRoot = id
			reopened = true
			if th != nil {
				reopenAgent = th.Agent
			}
			if th != nil && th.Client != thread.WebClient {
				elsewhere = inbox.ConversationView(accountID, th)
			} else {
				cfg.ContextID = id
				cfg.InitialConvHTML = renderThreadTurns(accountID, id)
				// A run still in flight when this page loaded. See
				// agent.Pending: the component shows that it is working and
				// polls until the answer lands, rather than leaving the
				// question sitting there with nothing under it.
				cfg.Pending = Pending(accountID, id)
			}
		}
	}

	// Prefill prompt from ?q / ?prompt (e.g. home card deep-links).
	//
	// The one query that stays a query, and the reason is that here the URL *is*
	// the message: a link that asks a question has nowhere else to carry it, and
	// somebody constructed and shared this one deliberately. It is not a search
	// box — the box on this page posts. See AGENTS.md, "What may travel in a URL".
	//
	// It is taken out of the address bar once it has been used — see the
	// replaceState below, and the same on Home — so it does not sit in the
	// history of whoever followed the link. That closes one of the four places a
	// URL comes to rest. The instance's own access log is another and records the
	// path only. The reverse proxy's log is the one that stays open, and it is
	// the price of a link that asks a question at all.
	prefill := r.URL.Query().Get("prompt")
	if prefill == "" {
		prefill = r.URL.Query().Get("q")
	}

	// Which agent this page is for. Clicking an agent's name on /agents means
	// "talk to that one", so it selects the agent, opens its most recent
	// conversation, and shows only that agent's conversations in the rail —
	// which is what a chat with one agent means. Before, it selected the agent
	// and then dropped you into whichever conversation happened to be newest,
	// listing every agent's history beside it.
	selAgent := r.URL.Query().Get("id")
	if selAgent == "" {
		selAgent = r.URL.Query().Get("agent")
	}
	// /agent/<name> — the page is about one agent, and says so in its address.
	//
	// named is what removes the roster from the room: on a page about one agent
	// the list of the others is the same furniture as a services list down the
	// side of /mail.
	named := false
	if slug := strings.TrimPrefix(r.URL.Path, "/agent/"); r.URL.Path != "/agent" &&
		slug != "" && !strings.Contains(slug, "/") {
		id, ok := agentSlugTarget(accountID, slug)
		if !ok {
			app.NotFound(w, r, "no agent called "+slug)
			return
		}
		selAgent, named = id, true
	}
	if reopened {
		// A reopened conversation decides its own agent; the rail filters to it.
		selAgent = reopenAgent
	} else if selAgent != "" && prefill == "" {
		// Land in the last conversation with this agent, if there is one.
		if last := latestThreadFor(accountID, selAgent, named); last != "" {
			cfg.ContextID = last
			cfg.InitialConvHTML = renderThreadTurns(accountID, last)
			cfg.Pending = Pending(accountID, last)
			activeRoot = last
		}
	}

	// What to ask, in this agent's words. The box read "Try: give me a morning
	// brief" on every page including this one, so the Weather agent's own page
	// suggested the general agent's work — see micro.Agent.Examples.
	if ex := examplesFor(selAgent); len(ex) > 0 {
		cfg.Suggestions = ex
	}

	// One panel beside the conversation: the conversations. On a phone it folds
	// away behind the bar below and the chat is the first thing on the page —
	// see chatLayoutCSS.
	//
	// The roster used to be the other one, and it is gone from here. /agents is
	// the page that lists them and links to each one's chat, connect and edit;
	// carrying a second copy of it down the side of every agent's own page is
	// the room holding the list of rooms. The back link above the conversation
	// is how you leave, which is the same shape /agent/connect already had.
	//
	// It was also load-bearing in a way nothing said out loud: renderAgentsPanel
	// carried the <script> defining muAgentCsrf and window.muSeedAgent, and it
	// was only emitted when !named — while this page calls muSeedAgent
	// unconditionally and the conversation rail calls muAgentCsrf to delete. So
	// on /agent/<name> both threw ReferenceError, which is why the delete cross
	// did nothing and why a named agent's page did not look like the default
	// one. What this page needs it now defines itself, in chatPageJS.
	rail := `<div class="chat-side">` +
		`<div class="chat-pane" id="pane-chats">` +
		renderSessionsRail(accountID, activeRoot, selAgent, named) + `</div></div>`

	// The bar above the conversation: how you got here, and how to see the
	// other conversations on a phone. Nothing else.
	//
	// It held three more things and all three have gone the same way — they
	// were a second copy of something /agents already does better.
	//
	//   - A chip naming the agent, which is the page's own title one line up.
	//   - A Connect link. /agents lists every agent with chat, connect and edit
	//     beside each, so the way to reach this agent's Connect page is the way
	//     you reached this one.
	//   - A picker opening the roster in the rail, which is the room carrying
	//     the list of rooms.
	//
	// Nothing replaces them. The bar is the phone control for the conversations
	// and nothing else, so on a desktop it is empty and takes no room — which
	// is the point: the box you came to type in starts at the top of the page.
	//
	// A back link went here briefly and it was the wrong shape for this page.
	// Leaving is what the sidebar is for, on every other page too; a link that
	// only exists here put a row of chrome above the conversation to say
	// something the nav already says.
	chip := `<div class="agent-bar">` +
		`<button type="button" class="chat-open-list" onclick="muPane('chats')">Chats</button>` +
		`</div>` + paneJS

	// No tabs. There were four — Chat, Threads, Runs, Connect — for one thing:
	// you, an agent, and what you have said to each other. Two of them listed
	// the same conversations under different headings, one listed the workflow
	// records behind them, and the strip changed shape as you moved between
	// agents. This is the page; Connect is the link in the bar above, which was
	// already there and says what it does.
	// This page is for talking to it, so the box asks rather than searches.
	// See app.ChatConfig.Ask — search is the default everywhere else, because
	// search works with no model and a page about an agent obviously does not.
	cfg.Transcript = true
	cfg.Ask = true
	main := app.ChatComponent(cfg)
	if elsewhere != "" {
		main = elsewhere
	}
	content := `<div class="chat-layout">` + rail + `<div class="chat-main">` + chip + main +
		`</div></div>` + chatLayoutCSS + chatPageJS

	// Seed the active agent so the panel highlights it and follow-ups continue
	// with it: an explicit ?id= selection (deep link) wins; otherwise a reopened
	// session restores the agent it was using (blank resets to default,
	// overriding any stale in-tab selection).
	//
	// ?id= because that is what /agent/new and /agents already call an agent's
	// id, and the same object should not have two names depending on which page
	// links to it. ?agent= still works: links to it exist, and breaking a URL to
	// tidy a parameter is a bad trade.
	// The id stays in the URL. It used to be stripped straight back out with
	// replaceState, which made the address bar disagree with the page and a
	// reload forget which agent you were talking to — the redirect looked
	// wasteful because it was.
	//
	// Seeded unconditionally, including the empty default. selAgent is what the
	// rail was filtered by and what the conversation was loaded for, so it is
	// the page's answer to "which agent is this". Seeding only when it was
	// non-empty left the tab's remembered selection in charge on a bare /agent:
	// the rail listed every agent's conversations while the next message went to
	// whichever agent the tab remembered. The URL is the state.
	content += `<script>window.muSeedAgent(` + app.JSString(selAgent) + `);</script>`
	if prefill != "" {
		content += `<script>(function(){var i=document.getElementById('mu-chat-input');if(i&&window.muChatAsk){i.value=` + app.JSString(prefill) + `;window.muChatAsk(i.value);}history.replaceState(null,'',` + app.JSString(Path(accountID, selAgent)) + `);})()</script>`
	}

	// The agent's own name, because this page is about one agent.
	//
	// It said "Inbox", from when this rail was the inbox and there was no other
	// page called that. There is now — /inbox is the mailbox, and this is where
	// you talk to an agent — so the title was the clearest possible statement
	// that the two had not been told apart.
	title := agentTitle(accountID, selAgent)
	desc := "Talk to " + title + ", and the address it answers on"
	app.Respond(w, r, app.Response{Title: title, Description: desc, HTML: content})
}

// openThread resolves whatever is in a ?session= link to a conversation.
//
// Three things have been called a session id: a thread id, which is what the
// rail links with now; the root of a chain of workflow records, which is what it
// linked with before; and any turn in the middle of one, which is what older
// bookmarks and the /agent/flow redirect hold. The last two are the same lookup,
// because the web keyed its conversation on the chain's root.
//
// A chain with no conversation behind it predates the record, and is adopted
// into it here rather than rendered from workflow records — otherwise the chat
// would keep a second way of reading a conversation forever, which is the thing
// being fixed.
func openThread(accountID, id string) string {
	if th := thread.Get(accountID, id); th != nil {
		return th.ID
	}
	chain := sessionChain(accountID, id)
	if len(chain) == 0 || chain[len(chain)-1].AccountID != accountID {
		return ""
	}
	return adopt(accountID, chain)
}

// renderThreadTurns renders a conversation into the chat log, most recent
// inbox.MessagesShown of it — see the constant.
func renderThreadTurns(accountID, threadID string) string {
	var b strings.Builder
	for _, m := range thread.Messages(accountID, threadID, inbox.MessagesShown) {
		b.WriteString(renderTurn(m))
	}
	return b.String()
}

// latestThreadFor is the most recent conversation with an agent on this page.
func latestThreadFor(accountID, agentID string, named bool) string {
	for _, t := range chatThreads(accountID, agentID, named) { // newest first
		return t.ID
	}
	return ""
}

// railShown bounds the rail.
//
// A conversation list is not a page of results, it is somewhere you glance: the
// one you want is almost always in the last few, and the rest is what /recall
// searches. Unbounded, it grew with the account and rendered every row of it on
// every page load.
const railShown = 40

// chatThreads is what belongs in the rail: the conversations you have had with
// this agent here.
//
// Here, and not everywhere. The rail listed every conversation on the account
// whichever client carried it — which is exactly what /inbox lists, so the two
// pages were two lists of the same thing with different furniture, and neither
// could be described in a sentence. The line that holds is the one every mail
// product draws: an inbox is what arrived, a chat is what you started. See
// thread.Arrived.
//
// So a mail chain, a WhatsApp exchange and this morning's briefing are on
// /inbox, where you deal with what came in. What is here is the chat: the
// conversations you opened on this instance's own screens, were present for,
// and watched the answer to.
func chatThreads(accountID, agentID string, named bool) []thread.Thread {
	var out []thread.Thread
	for _, t := range thread.List(accountID, 0) {
		if thread.Arrived(t) {
			continue
		}
		// One agent's conversations, when the page is about one agent.
		//
		// The filter used to be `agentID != ""`, and the default agent's id *is*
		// the empty string — so on /agent/micro, which is one agent's page, it
		// filtered nothing and listed every conversation on the account. Opening
		// a chat with Micro showed Foobar's history first, and clicking it
		// switched the agent, because a reopened conversation carries its own.
		//
		// Same shape as the scope bug in filterServices: empty meant both "no
		// restriction" and "the default", and the two are different statements.
		// named is which one this is — the path said an agent, whoever it is.
		if named && t.Agent != agentID {
			continue
		}
		out = append(out, t)
		if len(out) >= railShown {
			break
		}
	}
	return out
}

// inboxAddress is where this agent can be written to, or "" when the instance
// has no mail domain and there is nothing to advertise.
func inboxAddress(accountID, agentID string) string {
	if agentID != "" {
		if a := For(accountID, agentID); a != nil {
			return a.Address()
		}
		// One of this instance's own. Its address is agent+<name>@, which is
		// what /agents publishes — and this returned the bare agent@ for all of
		// them, so the list said "write to agent+weather@" and the weather
		// agent's own page said to write to the catch-all. Two addresses for
		// one agent, and the one on its page reaches a different agent.
		if platformName(agentID) != "" {
			if at := PlatformAddress(agentID); at != "" {
				return at
			}
		}
	}
	return mail.SharedAgentAddress()
}

// renderSessionsRail renders the inbox: every conversation, whichever way it
// arrived.
//
// It is a list of threads with an address behind it, which is an inbox — and
// calling it one is not a rename for its own sake. The rail was a list of chats
// on a page, and what it actually holds is every conversation this account has
// had with an agent, on any client, most of which did not start here.
func renderSessionsRail(accountID, currentID, agentID string, named bool) string {
	sessions := chatThreads(accountID, agentID, named)
	// A new chat with the agent whose rail this is. It used to rewrite the URL
	// to a bare /agent, which dropped the agent out of the address bar while
	// the page went on talking to it — so a reload landed you on the default
	// and the rail silently widened to every conversation on the account.
	// The agent's own page. It rewrote the URL to /inbox, which is a different
	// page now — the rail holds the conversations you started here and /inbox
	// holds what arrived, so a link from one to the other lands somewhere that
	// does not have the conversation in it.
	base := Path(accountID, agentID)
	newURL := base
	if agentID != "" && base == "/agent/"+DefaultSlug {
		// An id that resolves to nothing in the roster. Keep it in the URL
		// rather than silently rewriting to the default, which is what widened
		// the rail to the whole account.
		newURL = "/agent?id=" + url.QueryEscape(agentID)
	}
	var b strings.Builder
	b.WriteString(`<aside class="chat-rail"><button class="btn chat-new" onclick="if(window.muChatNew){muChatNew();history.replaceState(null,''` +
		`,` + app.JSAttr(newURL) + `);document.querySelectorAll('.chat-sess.active').forEach(function(e){e.classList.remove('active')});}">New</button>` +
		`<div class="chat-sess-scroll">` +
		// Chats, which is what the store has always called them in every way
		// but this one. The record is threads.json, the package is
		// internal/thread, the types are thread.Thread and thread.Message —
		// "Conversations" appeared in exactly one place, this heading, and was
		// a word the UI invented for something the code names twice over.
		//
		// Not "Sessions": auth.Session is a login, and reusing it here would
		// make the one word in this product that means "you are signed in"
		// also mean "a chat you had". Chat is what these are, it is what the
		// button under them starts, and thread.WebClient is the client that
		// makes them.
		`<div class="chat-sess-head">Chats</div><div class="chat-sess-list">`)
	if len(sessions) == 0 {
		// An empty inbox says how to fill it, and the answer is an address.
		// "No conversations yet" is a true sentence that leaves somebody looking
		// at a blank column with nothing to do about it — and the thing they can
		// do is the whole product: write to it from anywhere.
		where := ""
		if a := inboxAddress(accountID, agentID); a != "" {
			where = ` Or write to it at <code>` + htmlEsc(a) + `</code> — from your ` +
				`mail, from your phone, from anywhere that can send an email.`
		}
		if agentID != "" {
			b.WriteString(`<div class="chat-sess-empty">Nothing here yet. Ask this agent ` +
				`something.` + where + `</div>`)
		} else {
			b.WriteString(`<div class="chat-sess-empty">Nothing here yet. Ask something ` +
				`below.` + where + `</div>`)
		}
	}
	for _, s := range sessions {
		cls := "chat-sess"
		if s.ID == currentID {
			cls += " active"
		}
		title := s.Subject
		if title == "" {
			title = "Untitled"
		}
		if len(title) > 60 {
			title = title[:60] + "…"
		}
		// Where it happened, when that is not here. The common case carries no
		// chip: every row saying "web" is noise, and the point of the mark is
		// that a conversation you cannot continue from this page should not look
		// like one you can.
		where := ""
		if s.Client != thread.WebClient {
			// The same pill the inbox draws. It was .chat-sess-where here and
			// .ib-tag there, two names and two stylesheets for one shape.
			where = ` ` + app.Pill(app.ClientName(s.Client))
		}
		// Deletable. A conversation you can start and never be rid of is a list
		// that only grows, and the rail is the one place somebody looks at it.
		b.WriteString(`<div class="chat-sess-row has-row-del"><a href="` + base + `?session=` + url.QueryEscape(s.ID) +
			`" class="` + cls + `">` + htmlEsc(title) + where + `</a>` +
			`<button type="button" class="row-del" title="Delete conversation" ` +
			`aria-label="Delete this conversation" ` +
			`onclick="muSessionDelete(` + app.JSAttr(s.ID) + `,event)">&times;</button></div>`)
	}
	if len(sessions) >= railShown {
		b.WriteString(`<a class="chat-sess-more" href="/recall">Older conversations →</a>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`</div>` + sessionDeleteJS(base) + `</aside>`)
	return b.String()
}

// sessionDeleteJS removes a conversation and its record. DELETE on the flow
// path, which is where a conversation is already deleted from — the id is the
// chain's root and the server walks the rest.
func sessionDeleteJS(back string) string {
	return `<script>
// The conversation. It used to DELETE a workflow record, because the rail was
// built out of those.
function muSessionDelete(id,ev){
  ev.preventDefault();ev.stopPropagation();
  if(!confirm('Delete this conversation? What was said in it is gone.'))return;
  fetch('/agent/session/'+encodeURIComponent(id),{method:'DELETE',headers:{'X-CSRF-Token':muAgentCsrf()}})
    .then(function(){window.location=` + app.JSString(back) + `;});
}
// A conversation that has just started, listed while you are still in it.
//
// The rail is rendered by the server, so a new conversation appeared only on
// the next page load: you said hello, got an answer, refreshed for some other
// reason and a row you had never seen was suddenly there. The chat calls this
// when the first turn of a fresh conversation comes back.
window.muSessionStarted=function(id,title){
  var list=document.querySelector('.chat-sess-list');if(!list)return;
  var empty=list.querySelector('.chat-sess-empty');if(empty)empty.remove();
  var href='/agent?session='+id;
  if(list.querySelector('a[href="'+href+'"]'))return;
  title=(title||'Untitled').trim();
  if(title.length>60)title=title.slice(0,60)+'…';
  list.querySelectorAll('.chat-sess.active').forEach(function(e){e.classList.remove('active');});
  var row=document.createElement('div');row.className='chat-sess-row has-row-del';
  var a=document.createElement('a');a.className='chat-sess active';a.href=href;a.textContent=title;
  var b=document.createElement('button');b.className='row-del';b.type='button';
  b.title='Delete conversation';b.textContent='×';
  b.onclick=function(e){muSessionDelete(id,e);};
  row.appendChild(a);row.appendChild(b);
  list.insertBefore(row,list.firstChild);
};
</script>`
}

// paneJS opens one of the side panels as a sheet on a phone.
//
// Nothing here runs on a desktop: the buttons that call it are hidden and the
// panels are always visible, so the class it toggles matches nothing.
const paneJS = `<script>
function muPane(which){
  var el=document.getElementById('pane-'+which),side=document.querySelector('.chat-side');
  if(!el||!side)return;
  var wasOpen=el.classList.contains('open');
  document.querySelectorAll('.chat-pane.open').forEach(function(p){p.classList.remove('open')});
  if(!wasOpen)el.classList.add('open');
  var any=!!document.querySelector('.chat-pane.open');
  side.classList.toggle('up',any);
  var scrim=document.querySelector('.chat-scrim');
  if(!scrim){
    scrim=document.createElement('div');scrim.className='chat-scrim';
    scrim.onclick=function(){muPaneClose()};
    document.body.appendChild(scrim);
  }
  scrim.classList.toggle('up',any);
}
function muPaneClose(){
  document.querySelectorAll('.chat-pane.open').forEach(function(p){p.classList.remove('open')});
  var side=document.querySelector('.chat-side');if(side)side.classList.remove('up');
  // Removed, not un-classed.
  //
  // The scrim is appended to document.body, which outlives the page: this site
  // navigates softly, replacing #content and leaving body's other children
  // alone. So a sheet opened on a phone and then left by tapping a
  // conversation took its grey overlay to the next page and kept it there,
  // with nothing on screen to dismiss it — reported as "once I select a chat
  // the screen stays grey". Taking it out of the DOM cannot leave it behind.
  var scrim=document.querySelector('.chat-scrim');if(scrim)scrim.remove();
}
document.addEventListener('keydown',function(e){if(e.key==='Escape')muPaneClose()});
// Picking anything inside a sheet closes it. A conversation is a link, and a
// link that navigates while the sheet is still up leaves both behind.
document.addEventListener('click',function(e){
  var a=e.target.closest&&e.target.closest('.chat-pane a');
  if(a)muPaneClose();
},true);
</script>`

// chatPageJS is what the conversation page calls, on the page that calls it.
//
// These two lived in renderAgentsPanel's <script>, which this page emitted only
// when it was not about a named agent — while calling both regardless. On
// /agent/<name> that was three failures from one missing script:
//
//   - window.muSeedAgent threw, so window.muActiveAgent was never set. The chat
//     component sends it with every message (see internal/app/chat.go) and the
//     server has no fallback to the URL, so a question asked on an agent's own
//     page was answered by the default agent.
//   - muAgentCsrf threw, so the delete cross on a conversation did nothing.
//   - The throw aborted the rest of that inline script, which is why a named
//     agent's page did not lay out like the default one.
//
// Guarded rather than unconditional, because /agents and /agent/connect still
// render the panel and its versions know about things this page does not have
// — a roster to highlight, a chip to relabel. Where both are present the
// richer one wins; where only this is, the page still works.
const chatPageJS = `<script>
(function(){
  var K='mu_active_agent';
  if(typeof window.muActiveAgent==='undefined'){
    try{window.muActiveAgent=sessionStorage.getItem(K)||'';}catch(e){window.muActiveAgent='';}
  }
  if(typeof window.muSeedAgent!=='function'){
    window.muSeedAgent=function(id){
      window.muActiveAgent=id||'';
      try{sessionStorage.setItem(K,window.muActiveAgent);}catch(e){}
    };
  }
  if(typeof window.muAgentCsrf!=='function'){
    window.muAgentCsrf=function(){
      var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);
      return m?decodeURIComponent(m[1]):'';
    };
  }
  // How much page is above the conversation, measured rather than guessed.
  //
  // .chat-layout is one screen tall: height:calc(100vh - var(--chat-chrome)).
  // Nothing has ever set --chat-chrome. It was referenced once, in its own
  // fallback, so every instance has used 120px — and the chrome above this
  // layout is 192px, so the column has always been 72px taller than the window.
  //
  // That was invisible while the rail held one scrolling list: the overflow was
  // empty space below a scroll region, clipped by .chat-side's overflow:hidden
  // and noticed by nobody. Then the rail grew a second section pinned under the
  // first, and the 72px that falls off the bottom of the screen became the
  // thing somebody is trying to read. Reported as a section overlapping the
  // bottom of the conversations.
  //
  // Measured on every load and resize, because the number is a fact about the
  // rendered page — the nav wraps at some widths, a banner appears — and any
  // constant here is wrong again the next time something above it changes.
  function muChatChrome(){
    var el=document.querySelector('.chat-layout');
    if(!el)return;
    // Its distance from the top of the document, not of the viewport: the page
    // may be scrolled when this runs.
    var top=el.getBoundingClientRect().top+(window.scrollY||0);
    document.documentElement.style.setProperty('--chat-chrome',Math.max(0,Math.round(top))+'px');
  }
  muChatChrome();
  window.addEventListener('resize',muChatChrome);
  // Fonts land after first paint and move everything above the layout down.
  if(document.fonts&&document.fonts.ready)document.fonts.ready.then(muChatChrome);

  // A scrim left behind by the page before this one. Soft navigation keeps
  // body's children, so without this the grey survives a reload of the layout.
  document.querySelectorAll('.chat-scrim').forEach(function(s){s.remove();});
})();
</script>`

const chatLayoutCSS = `<style>
.chat-layout{display:flex;gap:24px;align-items:flex-start}
.chat-side{width:250px;flex-shrink:0;display:flex;flex-direction:column}

/* On a desktop the page does not scroll; the two columns do.
 *
 * A conversation and a list of conversations both grow without limit, and when
 * the page carries that growth the box you came to type in leaves the screen.
 * So the layout is one screen high and each column keeps its own overflow,
 * which is what every mail client does and for this reason.
 *
 * All of it in a min-width block, and none of it in the base rules. Height on
 * a flex chain is all-or-nothing — .chat-sess-list scrolls only if every
 * ancestor between it and .chat-layout refuses to grow — and putting those
 * properties in the base rules applied them to the phone sheet too, where the
 * chain is different and a min-height:0 collapsed the list to nothing. The
 * sheet sizes itself; it wants none of this.
 */
@media(min-width:761px){
  .chat-layout{height:calc(100vh - var(--chat-chrome, 120px));min-height:420px}
  .chat-side{min-height:0;max-height:100%;overflow:hidden}
  /* The link that was missing: a plain div between the column and the rail,
     which grew to its content and pushed the page down while the list below it
     carried an overflow rule that could never fire. */
  .chat-side>.chat-pane{flex:1 1 auto;min-height:0;display:flex;flex-direction:column}
  /* flex-shrink:0 on the rail is about the horizontal axis — it is a column
     beside the conversation and must not be squeezed narrow. As a column child
     of the pane it applies to height instead, which pinned the rail at its full
     content height and let it be clipped rather than scrolled: the rows past
     the fold were unreachable, which is worse than a long page. */
  .chat-side .chat-rail{flex:1 1 auto;min-height:0;display:flex;flex-direction:column}
  .chat-main{min-height:0;display:flex;flex-direction:column;overflow-y:auto}
  /* The rail scrolls once, below the New button — not once per list.
   *
   * This was .chat-sess-list{overflow-y:auto}, written when the rail held
   * exactly one list. It now holds two, Conversations and what the agent
   * answered in the inbox, and two auto-height scrollers sharing a fixed-height
   * column both size to their own content: together they overrun the rail, and
   * the second is drawn over the bottom of the first. Reported as a section
   * overlapping the conversations.
   *
   * A scroll region is a property of the column, not of each list in it. One
   * wrapper carries it, the New button stays put above, and a third section
   * added later cannot bring the overlap back. */
  .chat-sess-scroll{flex:1 1 auto;min-height:0;overflow-y:auto}
}
.chat-side .chat-rail{width:auto}
.chat-rail{width:250px;flex-shrink:0}
.chat-main{flex:1;min-width:0}
/* The conversation fills the main pane here (the 760px readability cap only
   applies to the chat embedded on the landing/home page). */
.chat-main #mu-chat{max-width:none}
/* The bar above the conversation: the way back, and on a phone the way to the
   other conversations. It held a chip naming this agent and a Connect link
   too; both were a second copy of what /agents does, so both are gone and the
   rules for them with them. Same shape as .conn-back on /agent/connect. */
/* The bar holds one control and that control is phone-only, so on a desktop
   this is an empty flex row 14px tall with 14px of margin under it — 38px of
   nothing above the input, which put the box lower than the + New button
   beside it and made the two columns start at different heights. Measured:
   the button at y=110, the input at y=148.
   Hidden here and shown in the phone query below, rather than left to lay
   out around a child that is not there. */
.agent-bar{display:none;align-items:center;gap:12px;flex-wrap:wrap;margin:0 0 14px;font-size:13px}
.chat-sess-head{font-size:11px;font-weight:600;letter-spacing:.04em;text-transform:uppercase;
  color:var(--text-muted,#999);padding:0 10px 6px}
/* Tokens, not hex.
   Every colour in this rail was a literal — #444 on no background at all for a
   row, #eef0ff under #111 for the selected one — so the rail did not move with
   the rest of the palette, and a row that declares a text colour and inherits
   its background is the exact shape that goes unreadable when anything
   underneath it changes. */
/* Size comes from the control scale in mu.css; this is the one thing that
   is particular to it — a new-conversation button spans its column. */
.chat-new{width:100%;margin-bottom:12px}
.chat-sess-list{display:flex;flex-direction:column;gap:2px}
.chat-sess-row{display:flex;align-items:center;gap:6px;min-width:0}
.chat-sess{display:block;flex:1;min-width:0;padding:8px 10px;border-radius:6px;
  background:var(--card-background,#fff);color:var(--text-secondary,#444);
  text-decoration:none;font-size:13px;line-height:1.35;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.chat-sess:hover{background:var(--hover-background,#f5f5f5)}
.chat-sess.active{background:var(--active-background,#eef0ff);color:var(--text-primary,#111);font-weight:600}
/* Deleting a conversation is .row-del in mu.css — the same control the inbox
   draws, rather than a third private one. The row marks itself with
   .has-row-del so the generic hover rule finds it; nothing else is needed
   here. */
.chat-sess-more{display:block;padding:8px 10px;font-size:12px;color:var(--text-muted,#888);text-decoration:none}
.chat-sess-more:hover{color:var(--text-primary,#111)}
.chat-sess-empty{color:var(--text-muted,#999);font-size:13px;padding:8px 10px;line-height:1.5}
.chat-sess-empty code{background:var(--hover-background,#f4f4f5);border-radius:4px;padding:1px 5px;font-size:11px;overflow-wrap:anywhere}
/* The two buttons in the bar are for phones. On a desktop the panels are always
   there, so a control that opens them is a second way to do nothing — and
   "Agent: Micro" beside a page whose title is Micro is the same word twice. */
.chat-open-list{display:none}

/* On a phone the conversation is the page.
   It was a stacked column: the agent picker first, then every conversation as a
   row of horizontally-scrolling chips, and only then the box you came to type
   in — so the first screen of the agent page was two lists and no agent. Now the
   panels are a sheet that slides up from the bottom when you ask for one, and
   the chat starts at the top.
   Stacking also turns align-items:flex-start into *horizontal* alignment, so
   children size to their content rather than the screen and a wide answer pushes
   the input off the side. Hence the stretch. */
@media(max-width:760px){
  .agent-bar{display:flex}
  .chat-layout{flex-direction:column;gap:12px;align-items:stretch}
  .chat-main{width:100%;min-width:0;max-width:100%}
  .chat-open-list{display:inline-block;border:1px solid var(--border-color,#e5e5e5);
    background:var(--card-background,#fff);color:var(--text-primary,#111);
    border-radius:6px;padding:3px 12px;font-size:12px;font-weight:600;
    font-family:inherit;cursor:pointer}
  .chat-open-list::after{content:" ▾";color:#999}
  /* The sheet. Off-screen rather than display:none, so opening it animates and
     so the panels inside keep their state. */
  /* The sheet ends above the tab bar. It is fixed to bottom:0 with a lower
     z-index than the bar, so without this the last conversation in the list is
     drawn underneath it and cannot be tapped. --tabbar already carries the
     safe-area inset, so it replaces it rather than adding to it. */
  .chat-side{position:fixed;left:0;right:0;bottom:0;max-height:72vh;overflow-y:auto;
    width:auto;padding:12px 14px calc(14px + var(--tabbar, env(safe-area-inset-bottom)));
    background:var(--card-background,#fff);border-top:1px solid var(--border-color,#e5e5e5);
    border-radius:14px 14px 0 0;box-shadow:0 -10px 30px rgba(0,0,0,.14);
    z-index:60;transform:translateY(101%);transition:transform .18s ease;visibility:hidden}
  .chat-side.up{transform:translateY(0);visibility:visible}
  .chat-scrim{position:fixed;inset:0;background:rgba(0,0,0,.28);z-index:59;display:none}
  .chat-scrim.up{display:block}
  .chat-pane{display:none}
  .chat-pane.open{display:block}
  .chat-rail{width:100%}
  /* A vertical list, which is what a list of conversations is. As a horizontal
     scroller everything past the third was off the side of the screen with
     nothing saying so. */
  .chat-sess-list{flex-direction:column;overflow:visible;flex-wrap:nowrap}
  /* Same reason as the desktop rule: the cap belongs to the sheet, not to each
     list in it, or two lists get 52vh each and the sheet is 104vh tall. */
  .chat-sess-scroll{max-height:52vh;overflow-y:auto}
  .chat-sess-row{flex-shrink:0;max-width:none}
  .chat-sess{font-size:14px;padding:10px}
  .agents-list>div{padding:10px 8px;font-size:14px}
  .agents-actions{opacity:1}
  .agents-actions a,.agents-actions button{padding:4px 6px;font-size:14px}
}
</style>`

// FormatAge returns a human-friendly string for an elapsed duration.
func FormatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// serveFlowPage renders a saved flow for viewing and sharing.
func serveFlowPage(w http.ResponseWriter, r *http.Request, id string) {
	f := getFlow(id)
	if f == nil {
		http.NotFound(w, r)
		return
	}

	// JSON polling endpoint for mobile recovery
	if app.WantsJSON(r) || r.URL.Query().Get("json") == "1" {
		app.RespondJSON(w, map[string]any{
			"id":     f.ID,
			"status": f.Status,
			"html":   f.HTML,
			"error":  f.Error,
			"prompt": f.Prompt,
		})
		return
	}

	var b strings.Builder

	// Show conversation history if this flow has a parent chain
	if f.ParentID != "" {
		history := getConversationHistory(f.ParentID, 5)
		for _, h := range history {
			b.WriteString(`<div class="card step-quote">`)
			b.WriteString(`<div class="text-xs text-muted mb-2">Previous question:</div>`)
			b.WriteString(`<div class="text-base semibold mb-3">` + htmlEsc(h.Prompt) + `</div>`)
			b.WriteString(`<div class="text-base">` + app.RenderString(h.Answer) + `</div>`)
			b.WriteString(`</div>`)
		}
	}

	b.WriteString(`<div class="card">`)
	b.WriteString(`<p class="text-xs text-muted m-0 mb-1">Saved query</p>`)
	b.WriteString(`<h3 class="m-0 mb-3">` + htmlEsc(f.Prompt) + `</h3>`)
	b.WriteString(`<p class="text-xs text-muted">` + f.CreatedAt.Format("2 January 2006, 15:04 UTC") + `</p>`)
	b.WriteString(`</div>`)

	// Render the stored answer
	if f.Answer != "" {
		rendered := app.RenderString(f.Answer)
		b.WriteString(`<div class="card" id="agent-response">` + rendered + `</div>`)
	}

	// Append typed cards from stored steps
	for _, step := range f.Steps {
		if card := renderResultCard(f.AccountID, step.Tool, step.Result, step.Args); card != "" {
			b.WriteString(card)
		}
	}

	// References
	if len(f.Steps) > 0 {
		b.WriteString(`<div class="card text-sm"><h4 class="m-0 mb-2 text-sm text-muted">References</h4>`)
		for _, step := range f.Steps {
			formatted := formatToolResult(step.Tool, step.Result, step.Args)
			b.WriteString(renderToolCallRef(step.Tool, step.Args, formatted))
		}
		b.WriteString(`</div>`)
	}

	// Actions — no card wrapper, just links
	b.WriteString(`<div class="d-flex gap-3 items-center mt-3 text-sm">`)
	b.WriteString(`<a href="/agent?continue=` + f.ID + `">Continue →</a>`)
	b.WriteString(`<a href="#" onclick="var u=location.href;if(navigator.share){navigator.share({url:u})}else if(navigator.clipboard){navigator.clipboard.writeText(u).then(function(){this.textContent='Copied!'}.bind(this))}else{prompt('Copy:',u)};return false;">Share</a>`)
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: "Agent", Description: "Saved agent query: " + htmlEsc(f.Prompt), HTML: b.String()})
}

// handleDeleteFlow handles DELETE /agent/flow/<id>.
func handleDeleteFlow(w http.ResponseWriter, r *http.Request, id string) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}
	// One run. Deleting a conversation is /agent/session/<id>: the two records
	// have different lifetimes and are deleted separately.
	if err := deleteFlow(acc.ID, id); err != nil {
		http.Error(w, `{"error":"failed to delete run"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sseHeartbeat is how often a quiet stream says something.
//
// Short enough to beat the shortest idle timeout in the usual path — thirty
// seconds is common in proxies and some browsers — and long enough that it is
// invisible.
const sseHeartbeat = 10 * time.Second

// sse writes a single Server-Sent Events data line and flushes.
func sse(w http.ResponseWriter, event map[string]any) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// streamNativeSSE drives the agent and translates its stream into the SSE
// events the chat UI expects (tool_start/tool_done, stream_start/stream_token,
// response, done).
//
// It used to return a bool saying whether it had handled the request, so the
// caller could fall back to a second pipeline when it had not. There is no
// second pipeline, so there is nothing to report: a failure is reported to the
// person who asked, on the stream they are already reading.
func streamNativeSSE(w http.ResponseWriter, accountID, prompt string, opts QueryOpts, flow *Flow, threadID string) {
	// One writer, and something on the wire while nothing is happening.
	//
	// The hooks below are called from whatever goroutine is running the tool,
	// so every write to w needs the lock — that was a race before there was a
	// heartbeat to race with.
	//
	// The heartbeat is why this exists. Between one tool finishing and the next
	// starting the model is thinking, which can be a minute, and in that minute
	// nothing was written at all. An idle connection is what proxies and
	// browsers close, so a long build ended as "network error" on a run that
	// was working perfectly — the answer arrived at a socket nobody was holding
	// any more. A comment line is not an event, so the client ignores it and
	// the connection stays up.
	var wmu sync.Mutex
	send := func(ev map[string]any) {
		wmu.Lock()
		defer wmu.Unlock()
		sse(w, ev)
	}
	beating := make(chan struct{})
	defer close(beating)
	go func() {
		t := time.NewTicker(sseHeartbeat)
		defer t.Stop()
		for {
			select {
			case <-beating:
				return
			case <-t.C:
				wmu.Lock()
				fmt.Fprint(w, ": still here\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				wmu.Unlock()
			}
		}
	}()

	streaming := false
	emitted := false
	var captured strings.Builder
	var nativeTools []string
	// The tool names behind those labels, for the run record.
	var nativeToolNames []string
	// Pairing a start with its end, by the provider's call id.
	//
	// It was keyed on the label, which is the same string for every command in
	// a build — so after the first one the screen went quiet and stayed on the
	// between-tools label for the rest of the run. Guarded by the same mutex as
	// the writes: these hooks are called from whichever goroutine is running
	// the tool.
	startedTools := map[string]bool{}
	endedTools := map[string]bool{}
	// A call with no id still has to pair with itself, so it gets one.
	var unnamed int
	keyOf := func(run ToolRun) string {
		if run.ID != "" {
			return run.ID
		}
		unnamed++
		return fmt.Sprintf("%s#%d", run.Name, unnamed)
	}

	// The hooks travel in opts now, so this handler translates events to SSE
	// frames and owns nothing else about the run.
	sopts := opts
	sopts.Stream = StreamHooks{
		ToolStart: func(run ToolRun) {
			wmu.Lock()
			key := keyOf(run)
			if startedTools[key] {
				wmu.Unlock()
				return
			}
			startedTools[key] = true
			emitted = true
			nativeTools = append(nativeTools, run.Label)
			nativeToolNames = append(nativeToolNames, NativeToolName(run.Name))
			wmu.Unlock()
			send(map[string]any{"type": "tool_start", "name": run.Label, "message": run.Label})
		},
		ToolEnd: func(run ToolRun) {
			wmu.Lock()
			key := keyOf(run)
			if endedTools[key] {
				wmu.Unlock()
				return
			}
			endedTools[key] = true
			wmu.Unlock()
			send(map[string]any{"type": "tool_done", "name": run.Label, "message": run.Label + " — done"})
		},
		Token: func(tok string) {
			if shouldHoldNativeNewsStreamTokens(prompt, nativeTools) {
				captured.WriteString(tok)
				return
			}
			if !streaming {
				streaming = true
				emitted = true
				send(map[string]any{"type": "stream_start"})
			}
			captured.WriteString(tok)
			send(map[string]any{"type": "stream_token", "token": tok})
		},
	}
	answer, err := runNative(accountID, prompt, sopts)
	if err != nil {
		// Whether anything was on screen already decides how it reads, not
		// whether it is reported. Mid-answer the error follows the tokens the
		// reader has been watching; before any output it is the whole message.
		if emitted {
			app.Log("agent", "stream error mid-answer: %v", err)
		} else {
			app.Log("agent", "stream failed before output: %v", err)
		}
		updateFlow(flow.ID, func(f *Flow) { f.Status = "error"; f.Error = err.Error() })
		send(map[string]any{"type": "error", "message": agentErrorMessage(err)})
		send(map[string]any{"type": "done"})
		return
	}

	if answer == "" {
		answer = app.StripLatexDollars(captured.String())
	}
	answer = completeNativeToolAnswer(answer, nativeTools)
	answer = app.NormalizeAnswerMarkdown(answer)
	if !streaming && shouldReplayFinalNativeAnswer(prompt, nativeTools, captured.Len()) {
		streaming = true
		emitted = true
		send(map[string]any{"type": "stream_start"})
		send(map[string]any{"type": "stream_token", "token": answer})
	}
	// What was said, in the record. The workflow record below is how it was
	// produced; this is the answer itself, and it is what the next message in
	// this conversation will be given as history.
	// No account, no record.
	//
	// A guest asking from the front page has no account, and everything below
	// this line is keyed on one — the conversation, the workflow record, the
	// steps. Writing them anyway produces rows owned by nobody: nothing lists
	// them, nothing can delete them, and the account they name is the empty
	// string. The guard is on the account rather than on a flag passed down,
	// because that is the actual invariant and it holds for every caller of
	// this function.
	rendered := app.RenderString(answer)
	html := `<div class="card" id="agent-response">` + rendered + `</div>`
	if accountID != "" {
		Answered(accountID, threadID, answer, flow.ID)
		updateFlow(flow.ID, func(f *Flow) {
			f.Answer = answer
			f.HTML = html
			f.Status = "done"
			// What it ran. The stream emitted tool_start and tool_done to the
			// browser and then dropped both on the floor, so a finished run
			// recorded an answer and no account of how it got there.
			for _, name := range nativeToolNames {
				f.Steps = append(f.Steps, FlowStep{Tool: name})
			}
		})
	}
	send(map[string]any{"type": "response", "html": html, "flow_id": flow.ID})
	send(map[string]any{"type": "done"})
}

// agentErrorMessage is what a failed run says to the person who asked.
//
// An unconfigured instance is the one failure worth telling apart, because it
// is the operator's to fix and every question will fail the same way until
// they do. Everything else is this run going wrong.
func agentErrorMessage(err error) string {
	if errors.Is(err, ErrNoProvider) {
		return "The agent has no AI provider configured. Set a provider key in /admin/config."
	}
	return "Could not generate response: " + err.Error()
}

// handleQuery processes an agent query request with SSE streaming.
func handleQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt    string `json:"prompt"`
		Model     string `json:"model"`
		Agent     string `json:"agent"`      // optional: user-defined agent id to answer as
		ContextID string `json:"context_id"` // optional: prior flow to continue from
		// Cards asks for the reader's home cards to be included as context, so
		// a question about what they watch is answered from what is already
		// known rather than fetched again.
		Cards bool `json:"cards"`
		// History is an optional client-supplied conversation thread used by
		// the inline chat (landing + assistant). It gives multi-turn context
		// without server-side persistence, so a conversation that predates the
		// record keeps its follow-up memory too.
		History []struct {
			Prompt string `json:"prompt"`
			Answer string `json:"answer"`
		} `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, `{"error":"prompt required"}`, http.StatusBadRequest)
		return
	}

	// An account, or a guest. See below for what a guest is allowed.
	_, acc := auth.TrySession(r)
	if acc == nil {
		w.Header().Set("Content-Type", "application/json")
		// A stranger can ask, which is the whole point of a front page.
		//
		// This was a flat 401 reading "Sign in to ask the agent", and the front
		// page's only control is this box — so every question a visitor typed
		// was answered with a sign-up form. ChatGPT answers without an account.
		// Claude answers without an account. Google has always answered without
		// an account. A personal server that refuses to say anything until you
		// register is asking for more than the products it is defined against.
		//
		// What a guest gets is bounded and it is public:
		//
		//   - Rate limited per IP, by the limiter that already existed for
		//     exactly this and was wired to nothing on this path. Localhost is
		//     never limited, so a self-hosted instance is unrestricted for the
		//     person running it.
		//   - Public tools only. QueryOpts.Public is set below, which drops
		//     every account-scoped service — mail, wallet, files, the inbox —
		//     through service.AccountScoped, the same policy the app SDK reads.
		//   - Nothing written down. No flow, no thread, no memory: there is no
		//     account to own any of it, and a record belonging to nobody is a
		//     record nobody can delete.
		//
		// An operator who does not want to pay for strangers sets
		// GUEST_MAX_PER_IP=0 and gets the old behaviour, stated below.
		if !app.GuestAllowed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"This instance is not answering strangers right now."}`)) //nolint:errcheck
			return
		}
	}

	// Charge up-front: the agent is our most expensive op, so it is metered
	// like web_fetch and chat.
	//
	// Empty for a guest, and every use of it below is guarded — the record is
	// keyed on an account and a guest has none.
	accountID := ""
	guest := acc == nil
	if acc != nil {
		accountID = acc.ID
	}

	// The conversation this message continues, in the system of record.
	//
	// context_id is a thread id: the page holds what the rail linked with and
	// sends it back. An id from before the record — the root of a chain of
	// workflow records, or any turn in one — resolves through the same door the
	// rail links through, so an open tab keeps working.
	//
	threadID := ""
	if !guest && req.ContextID != "" {
		threadID = openThread(accountID, req.ContextID)
	}

	// Load conversation history when continuing one. What was said, not how it
	// was produced — this used to walk the workflow chain, which is why an
	// evicted run silently truncated a conversation.
	var conversationHistory []*Flow
	if threadID != "" {
		conversationHistory = pastTurns(accountID, threadID, historyTurns)
	}
	if len(conversationHistory) == 0 && len(req.History) > 0 {
		const maxTurns = 6
		hist := req.History
		if len(hist) > maxTurns {
			hist = hist[len(hist)-maxTurns:]
		}
		for _, h := range hist {
			if strings.TrimSpace(h.Prompt) == "" {
				continue
			}
			ans := h.Answer
			if len(ans) > 1500 {
				ans = ans[:1500] + "…"
			}
			conversationHistory = append(conversationHistory, &Flow{Prompt: h.Prompt, Answer: ans})
		}
	}

	// Create flow early so progress is saved server-side even if the
	// SSE connection drops (e.g. mobile browser suspends the tab).
	flow := &Flow{
		ID:        newFlowID(),
		AccountID: accountID,
		Prompt:    req.Prompt,
		Status:    "running",
		Agent:     req.Agent,
		CreatedAt: time.Now().UTC(),
	}
	// The web drives its own run because it streams, so it cannot hand the whole
	// turn to Ask and wait — but it must not write its own version of the record
	// either, which is why it goes through the same three calls Ask uses.
	if !guest {
		if err := saveFlow(flow); err != nil {
			app.Log("agent", "Failed to create flow: %v", err)
		}
		if threadID == "" {
			// A conversation that starts here. Keyed on this first run, which is
			// unique and is what the record was already keyed on.
			threadID = Opened(accountID, thread.WebClient, flow.ID, "", req.Agent)
		}
		Said(accountID, threadID, req.Prompt, "", "")

		// Notice anything worth remembering. This ran only on /agent/run — the
		// REST and MCP path — so an agent remembered what a program told it and
		// forgot everything a person did, which is backwards: the chat is where
		// somebody says "I'm in London, keep it short". Cheap: one background
		// model call, off the response path, and it stores nothing when the
		// message holds no fact.
		go extractMemory(accountID, req.Prompt, scopeOf(req.Agent))
	}

	// Start SSE stream
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send both ids immediately: flow_id so the client can recover this run on
	// disconnect, thread so the next message continues this conversation. They
	// used to be the same id doing both jobs, which is how a run and a
	// conversation came to be one thing.
	sse(w, map[string]any{"type": "flow_id", "flow_id": flow.ID, "thread": threadID})

	nopts := QueryOpts{Public: guest}
	if !guest && req.Cards && CardContextFunc != nil {
		nopts.Extra = CardContextFunc(accountID)
	}
	if ua := resolveAgent(accountID, req.Agent); ua != nil && !guest {
		nopts.System = ua.SystemPrompt
		nopts.Tools = ua.Tools
	}
	// What was already said, from the record rather than from workflow
	// records. The history the page sent up is the fallback, for a
	// conversation that predates the record.
	if h := History(accountID, threadID, historyTurns); len(h) > 0 {
		nopts.History = h
	} else {
		for _, f := range conversationHistory {
			if strings.TrimSpace(f.Prompt) == "" {
				continue
			}
			nopts.History = append(nopts.History,
				QueryMessage{Role: "user", Text: f.Prompt},
				QueryMessage{Role: "assistant", Text: f.Answer})
		}
	}
	// The same routing decision every other client gets. It used to be
	// skipped entirely here, so the web answered as the generalist while
	// mail got a specialist — and because routing now sets the
	// options rather than diverting into a pipeline of its own, a routed
	// question still streams.
	routedPrompt, nopts := Routed(req.Prompt, nopts)
	streamNativeSSE(w, accountID, routedPrompt, nopts, flow, threadID)
}

func isLatestTechnologyNewsPrompt(lower string) bool {
	hasRecency := strings.Contains(lower, "latest") ||
		strings.Contains(lower, "today") ||
		strings.Contains(lower, "current") ||
		strings.Contains(lower, "happening")
	if !hasRecency || !strings.Contains(lower, "news") {
		return false
	}
	for _, topic := range []string{"tech", "technology", "ai", "artificial intelligence"} {
		if strings.Contains(lower, topic) {
			return true
		}
	}
	return false
}

func shouldReplayFinalNativeAnswer(prompt string, nativeTools []string, capturedLen int) bool {
	if capturedLen > 0 {
		return true
	}
	// streamNative buffers stale-news tokens internally once the tool payload
	// proves a freshness caveat is needed. In that case streamNativeSSE's local
	// capture is empty, so replay the guarded final answer as the first streamed
	// text instead of relying only on the later response replacement event.
	return shouldHoldNativeNewsStreamTokens(prompt, nativeTools)
}

func shouldHoldNativeNewsStreamTokens(prompt string, nativeTools []string) bool {
	if !isLatestTechnologyNewsPrompt(strings.ToLower(prompt)) {
		return false
	}
	// Latest-news prompts are sensitive to recency caveats: the native model can
	// emit answer text before the news.Search/dotted news tool payload has been
	// recorded and guarded. Hold the stream from the first token, then emit the
	// final guarded answer once completeToolAnswer has had a chance to prepend
	// stale/mostly-stale disclosures.
	if len(nativeTools) == 0 {
		return true
	}
	for _, tool := range nativeTools {
		lowerTool := strings.ToLower(strings.TrimSpace(tool))
		if canonicalToolTitle(lowerTool) == "news" || strings.Contains(lowerTool, "news") || strings.Contains(lowerTool, "headline") {
			return true
		}
	}
	return false
}

// toolLabel returns a human-readable progress label for a tool name.
func toolLabel(tool string) string {
	switch tool {
	case "news":
		return "📰 Reading latest news"
	case "news_headlines", "news_list":
		return "📰 Scanning headlines"
	case "news_read":
		return "📖 Reading article"
	case "news_search":
		return "Searching news"
	case "recall", "index":
		return "🧠 Searching your world"
	case "web_search", "search_web":
		return "🌐 Searching the web"
	case "web_fetch", "search_fetch":
		return "Fetching web page"
	case "video_search":
		return "🎬 Searching videos"
	case "markets", "markets_list":
		return "📈 Checking market prices"
	case "weather_forecast":
		return "🌤 Getting weather forecast"
	case "places_search":
		return "📍 Searching places"
	case "places_nearby":
		return "📍 Finding nearby places"
	case "prayer_reflection", "islam_today", "islam":
		return "📿 Getting today's reflection"
	case "search":
		return "Searching Mu"
	case "blog_list":
		return "📝 Reading blog posts"
	case "wallet_balance":
		return "💳 Checking wallet balance"
	case "apps_search":
		return "📱 Searching apps"
	case "apps_read":
		return "📱 Reading app"
	case "apps_build":
		return "🔨 Building app"
	case "apps_edit":
		return "✏️ Editing app"
	case "apps_embed":
		return "📱 Getting the embed code"
	default:
		return "⚙ Calling " + tool
	}
}

// renderToolCallRef renders a collapsible <details> element showing the tool
// name with arguments and the formatted result text, for use as a reference
// alongside the agent's synthesised answer.
func renderToolCallRef(name string, args map[string]any, formattedResult string) string {
	label := toolLabel(name)
	if args != nil {
		if q, ok := args["query"].(string); ok && q != "" {
			label += ` — "` + htmlEsc(q) + `"`
		} else if q, ok := args["q"].(string); ok && q != "" {
			label += ` — "` + htmlEsc(q) + `"`
		} else if cat, ok := args["category"].(string); ok && cat != "" {
			label += ` — ` + htmlEsc(cat)
		}
	}
	return `<details class="mb-1">` +
		`<summary class="step-toggle">` +
		label + `</summary>` +
		`<pre class="step-out">` +
		htmlEsc(formattedResult) + `</pre>` +
		`</details>`
}

// renderResultCard returns an HTML card to attach after the AI answer, or "" if
// the tool has no visual card.
// resolveAgent finds the agent a question is being asked as.
//
// It reads the roster first, which is where every agent made since /agents
// existed lives. The ask path used to read only agent/micro's user store — the
// store the roster replaced — so an agent you built, named and gave a system
// prompt was found by nothing when you asked it something: the lookup returned
// nil, the prompt and the tool scope were dropped, and the default assistant
// answered in its place. Nothing said so. That is the whole of "it is not clear
// whether a new agent is a real thing or a play thing": it was not a real
// thing, because the one place it had to be read was reading somewhere else.
//
// The old store is still consulted second, for anything the one-way import has
// not carried over.
func resolveAgent(accountID, id string) *micro.Agent {
	if accountID == "" || id == "" {
		return nil
	}
	if a := For(accountID, id); a != nil {
		return a.AsMicro()
	}
	// Somebody else's published agent used to resolve here, running on this
	// account with the author's standing instruction. It was the read half of a
	// directory nothing linked to — see the note on Agent in roster.go — and it
	// is gone with the rest of it. An id that is not yours resolves to nothing,
	// which is what an id you were never given should do.
	if a := micro.UserAgentFor(accountID, id); a != nil {
		return a
	}
	// And this instance's own specialists. Without this the chat resolved
	// agent+news@ to nothing and ran the default with every tool, so the page
	// named one agent and a different one answered.
	return micro.Get(id)
}

func renderResultCard(accountID, toolName, result string, args map[string]any) string {
	switch toolName {
	case "news", "news_search":
		return renderNewsCard(result)
	case "video_search":
		return renderVideoCard(result)
	case "places_search", "places_nearby":
		return renderPlacesCard(result, args)
	case "apps_search":
		return renderAppsCard(result)
	}
	// Service-sourced dashboard cards (markets, news_headlines, social, …),
	// pulled from the same tool registry, attached via api.SetCard in main.go.
	return api.CardForTool(toolName, service.For(accountID))
}

// --- typed card renderers ---

func renderNewsCard(result string) string {
	var data struct {
		Query     string     `json:"query"`
		Feed      []newsItem `json:"feed"`
		Results   []newsItem `json:"results"`
		Freshness struct {
			Status string `json:"status"`
			Notice string `json:"notice"`
		} `json:"freshness"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return ""
	}
	items := data.Results
	if len(items) == 0 {
		items = data.Feed
	}
	if len(items) == 0 {
		return ""
	}
	if newsCardQueryRequiresAI(data.Query) {
		items = filterNewsCardAIItems(items)
	}
	if len(items) == 0 {
		return ""
	}
	if len(items) > 5 {
		items = items[:5]
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>📰 News</h4>`)
	if notice := strings.TrimSpace(data.Freshness.Notice); notice != "" && (data.Freshness.Status == "stale" || data.Freshness.Status == "mostly_stale" || data.Freshness.Status == "no_dated_results") {
		b.WriteString(`<div class="notice warn">`)
		b.WriteString(htmlEsc(newsFreshnessCardNotice(data.Freshness.Status, notice)))
		b.WriteString(`</div>`)
	}
	for _, item := range items {
		link := item.URL
		if item.ID != "" {
			link = "/news?id=" + item.ID
		}
		b.WriteString(`<div class="thin-row row-pad">`)
		if item.Category != "" {
			b.WriteString(`<a href="/news#` + htmlEsc(item.Category) + `" class="category text-2xs mr-1">` + htmlEsc(item.Category) + `</a>`)
		}
		b.WriteString(`<a href="` + htmlEsc(link) + `" class="text-base semibold d-block">` + htmlEsc(item.Title) + `</a>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`<a href="/news" class="link d-inline-block mt-2">More news →</a></div>`)
	return b.String()
}

func newsFreshnessCardNotice(status, notice string) string {
	notice = strings.TrimSpace(notice)
	if status == "mostly_stale" {
		return userFacingNewsFreshnessSummary(notice)
	}
	return userFacingNewsFreshnessSummary(notice)
}

type newsItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	URL         string `json:"url"`
	Category    string `json:"category"`
}

func filterNewsCardAIItems(items []newsItem) []newsItem {
	filtered := items[:0]
	for _, item := range items {
		if newsCardAIItemIsBroadChipFinance(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func newsCardQueryRequiresAI(query string) bool {
	clean := strings.NewReplacer("-", " ", "_", " ", "/", " ", "'", "", "’", "").Replace(strings.ToLower(query))
	return newsCardTermMatches(clean, "ai") || strings.Contains(clean, "artificial intelligence") || strings.Contains(clean, "machine learning") || newsCardTermMatches(clean, "llm")
}

func newsCardAIItemIsBroadChipFinance(item newsItem) bool {
	haystack := strings.ToLower(strings.Join([]string{item.Title, item.Description, item.Content, item.Category}, " "))
	if haystack == "" {
		return false
	}
	hasChipFrame := false
	for _, term := range []string{"chip", "chips", "semiconductor", "sk hynix", "nvidia", "data center", "datacenter"} {
		if newsCardTermMatches(haystack, term) || strings.Contains(haystack, term) {
			hasChipFrame = true
			break
		}
	}
	if !hasChipFrame {
		return false
	}
	hasFinanceFrame := false
	for _, term := range []string{"market", "markets", "stock", "stocks", "shares", "trading", "investor", "investors", "nasdaq", "ipo", "valuation", "rally", "climb", "debut"} {
		if newsCardTermMatches(haystack, term) {
			hasFinanceFrame = true
			break
		}
	}
	if !hasFinanceFrame {
		return false
	}
	for _, term := range []string{
		"launch", "launches", "launched", "release", "releases", "released", "unveil", "unveils", "unveiled",
		"deploy", "deploys", "deployed", "deployment", "build", "builds", "built", "capacity", "processor",
		"accelerator", "gpu", "server", "inference", "training", "model serving", "model-serving", "cloud",
		"ai product", "ai model", "ai agent", "ai assistant", "ai safety", "ai governance", "ai infrastructure",
	} {
		if newsCardTermMatches(haystack, term) || strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func newsCardTermMatches(haystack, term string) bool {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return false
	}
	if strings.Contains(term, " ") {
		return strings.Contains(haystack, term)
	}
	for _, token := range strings.FieldsFunc(haystack, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if token == term {
			return true
		}
	}
	return false
}

func renderVideoCard(result string) string {
	var data struct {
		Results []videoItem `json:"results"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return ""
	}
	if len(data.Results) == 0 {
		return ""
	}
	items := data.Results
	if len(items) > 4 {
		items = items[:4]
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>🎬 Videos</h4>`)
	for _, v := range items {
		b.WriteString(`<div class="thin-row row-pad d-flex gap-3 row-top">`)
		if v.Thumbnail != "" {
			b.WriteString(`<img src="` + htmlEsc(v.Thumbnail) + `" class="thumb-sm" loading="lazy">`)
		}
		b.WriteString(`<div class="min-w-0"><a href="` + htmlEsc(v.URL) + `" class="text-sm semibold d-block">` + htmlEsc(v.Title) + `</a>`)
		if v.Channel != "" {
			b.WriteString(`<div class="text-2xs text-muted mt-px">` + htmlEsc(v.Channel) + `</div>`)
		}
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`<a href="/video" class="link d-inline-block mt-2">More videos →</a></div>`)
	return b.String()
}

type videoItem struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
	Channel   string `json:"channel"`
}

func renderPlacesCard(result string, args map[string]any) string {
	var data struct {
		Results []placeItem `json:"results"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return ""
	}
	if len(data.Results) == 0 {
		return ""
	}
	items := data.Results
	if len(items) > 5 {
		items = items[:5]
	}

	// Build a deterministic Google Maps search URL from the tool args so the
	// link opens the exact same query without any additional server-side cost.
	mapURL := placesMapURL(args, data.Results)

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>📍 Places</h4>`)
	for _, p := range items {
		b.WriteString(`<div class="thin-row">`)
		b.WriteString(`<div class="semibold">` + htmlEsc(p.Name) + `</div>`)
		if p.Category != "" || p.Address != "" {
			meta := p.Category
			if p.Address != "" {
				if meta != "" {
					meta += " · "
				}
				meta += p.Address
			}
			b.WriteString(`<div class="text-xs text-muted">` + htmlEsc(meta) + `</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<a href="` + htmlEsc(mapURL) + `" target="_blank" rel="noopener noreferrer" class="link d-inline-block mt-2">Open in Google Maps ↗</a></div>`)
	return b.String()
}

// placesMapURL builds a deterministic Google Maps search URL for the places
// results.  It prefers using the query/near tool args when available, falling
// back to a coordinate-based search centred on the first place result.
func placesMapURL(args map[string]any, items []placeItem) string {
	q := ""
	near := ""
	if args != nil {
		if v, ok := args["q"]; ok {
			q = fmt.Sprintf("%v", v)
		}
		if v, ok := args["near"]; ok {
			near = fmt.Sprintf("%v", v)
		}
		if near == "" {
			if v, ok := args["address"]; ok {
				near = fmt.Sprintf("%v", v)
			}
		}
	}

	if q != "" && near != "" {
		return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(q+" "+near)
	}
	if q != "" {
		return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(q)
	}

	// Fall back: centre on the first result with known coordinates.
	for _, p := range items {
		if p.Lat != 0 || p.Lon != 0 {
			return fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f", p.Lat, p.Lon)
		}
	}

	return "/places"
}

// formatToolResult converts a raw tool result into a human-readable text
// summary suitable for inclusion in the AI synthesis RAG context.
func formatToolResult(toolName, result string, args map[string]any) string {
	switch toolName {
	case "news", "news_search":
		return withCurrentDateContext(formatNewsResult(result))
	case "news_headlines", "news_list", "news_read":
		return withCurrentDateContext(result)
	case "video_search":
		return formatVideoResult(result)
	case "prayer_reflection", "islam_today", "islam":
		return formatReminderResult(result)
	case "search":
		return withCurrentDateContext(formatSearchResult(result))
	case "web_search", "search_web", "weather_forecast":
		return withCurrentDateContext(result)
	case "markets", "markets_list":
		return withCurrentDateContext(formatMarketsResult(result))
	case "web_fetch", "search_fetch":
		return formatWebFetchResult(result)
	case "places_search", "places_nearby":
		return formatPlacesResult(result, args)
	case "wallet_balance":
		return formatWalletBalanceResult(result)
	case "apps_search":
		return formatAppsSearchResult(result)
	case "apps_read":
		return formatAppsReadResult(result)
	case "apps_build":
		return formatAppsBuildResult(result)
	case "apps_edit":
		return formatAppsBuildResult(result) // same format: returns app details
	}
	return plainToolText(result)
}

// plainToolText unwraps the envelope every service answers in.
//
// The switch above names the tools somebody has written a formatter for, and
// everything else fell through as the literal bytes the tool returned — which
// for a go-micro service is `{"text":"..."}`, because that is the response shape
// the convention asks for. Most of the time nobody sees it: the model reads it,
// understands it perfectly well, and writes prose. But the freshness guard in
// answer_guard.go replaces a suspect answer with a rendering of the tool results
// themselves, and at that moment the raw envelope is the answer — which is how
// somebody asking their inbox to turn a sender down politely got
// `{"text":"Your inbox (5 messages):\n- ..."}` back as the agent's reply.
//
// So the default case unwraps rather than passing through. Doing it here rather
// than adding `mail_inbox` to the switch is the point: a list that has to be
// extended for every service written from now on is a list that will be missing
// the next one, and this is the shape they all share.
//
// Only a lone text field. An object with more in it — a price, a list, a
// structured answer — is left alone, because dropping its other fields would
// lose what a formatter is supposed to keep.
func plainToolText(result string) string {
	trimmed := strings.TrimSpace(result)
	if !strings.HasPrefix(trimmed, "{") {
		return result
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &body); err != nil {
		return result
	}
	raw, ok := body["text"]
	if !ok || len(body) != 1 {
		return result
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return result
	}
	if strings.TrimSpace(text) == "" {
		return result
	}
	return text
}

// formatMarketsResult converts raw markets tool JSON into readable price context
// for synthesis. The go-micro markets tool usually returns {"text":"..."},
// while the REST endpoint returns {"category":"...","data":[...]}; handle both so
// market-moving prompts never expose raw JSON to users or the model fallback.
func formatMarketsResult(result string) string {
	var textData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &textData); err == nil && strings.TrimSpace(textData.Text) != "" {
		return strings.TrimSpace(textData.Text)
	}

	var data struct {
		Category  string `json:"category"`
		UpdatedAt string `json:"updated_at"`
		Stale     bool   `json:"stale"`
		Partial   bool   `json:"partial"`
		Freshness string `json:"freshness"`
		Data      []struct {
			Symbol    string  `json:"symbol"`
			Price     float64 `json:"price"`
			Change24h float64 `json:"change_24h"`
			Type      string  `json:"type"`
			UpdatedAt string  `json:"updated_at"`
			Source    string  `json:"source"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Data) == 0 {
		return "No market prices available right now."
	}

	category := strings.TrimSpace(data.Category)
	if category == "" {
		category = "market"
	}

	var b strings.Builder
	if strings.TrimSpace(data.Freshness) != "" {
		fmt.Fprintf(&b, "%s.\n", strings.TrimSuffix(strings.TrimSpace(data.Freshness), "."))
	} else if strings.TrimSpace(data.UpdatedAt) != "" {
		fmt.Fprintf(&b, "Last refresh: %s.\n", strings.TrimSpace(data.UpdatedAt))
	}
	if data.Stale {
		b.WriteString("Disclosure: market data may be stale.\n")
	}
	if data.Partial {
		b.WriteString("Disclosure: some requested symbols are unavailable from the current source.\n")
	}
	fmt.Fprintf(&b, "Live %s prices:\n", category)
	count := 0
	for _, item := range data.Data {
		symbol := strings.TrimSpace(item.Symbol)
		if symbol == "" || item.Price == 0 {
			continue
		}
		if item.Change24h != 0 {
			fmt.Fprintf(&b, "%s: $%s (%+.2f%% 24h)\n", symbol, formatMarketPrice(item.Price), item.Change24h)
		} else {
			fmt.Fprintf(&b, "%s: $%s\n", symbol, formatMarketPrice(item.Price))
		}
		count++
	}
	if count == 0 {
		return fmt.Sprintf("No %s prices available right now.", category)
	}
	return strings.TrimSpace(b.String())
}

func formatMarketPrice(price float64) string {
	switch {
	case price >= 100:
		return fmt.Sprintf("%.2f", price)
	case price >= 1:
		return fmt.Sprintf("%.3f", price)
	default:
		return fmt.Sprintf("%.6f", price)
	}
}

// formatNewsResult converts a raw JSON news feed or search result into
// human-readable text for the AI synthesis RAG context.
func formatNewsResult(result string) string {
	var textData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &textData); err == nil && strings.TrimSpace(textData.Text) != "" {
		result = strings.TrimSpace(textData.Text)
	}

	var data struct {
		Feed []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Category    string `json:"category"`
			URL         string `json:"url"`
			Published   string `json:"published"`
			PostedAt    string `json:"posted_at"`
		} `json:"feed"`
		Results []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Category    string `json:"category"`
			URL         string `json:"url"`
			PostedAt    string `json:"posted_at"`
		} `json:"results"`
		Freshness struct {
			Status           string `json:"status"`
			Notice           string `json:"notice"`
			RequestedDate    string `json:"requested_date"`
			FreshestPostedAt string `json:"freshest_posted_at"`
		} `json:"freshness"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	var items []formattedNewsItem
	for _, a := range data.Results {
		items = append(items, formattedNewsItem{a.Title, a.Description, a.Category, a.URL, a.PostedAt, ""})
	}
	if len(items) == 0 {
		for _, a := range data.Feed {
			items = append(items, formattedNewsItem{a.Title, a.Description, a.Category, a.URL, a.PostedAt, a.Published})
		}
	}
	if len(items) == 0 {
		return "No news available."
	}

	// Freshness-sensitive searches feed both the fast tool fallback and the
	// native streaming/final synthesis paths. Keep dated current items ahead of
	// older replayed context before any model sees the result list.
	if data.Query != "" && (data.Freshness.Status == "mostly_stale" || data.Freshness.Status == "stale" || strings.TrimSpace(data.Freshness.Notice) != "") {
		sort.SliceStable(items, func(i, j int) bool {
			left := newsResultItemTime(items[i])
			right := newsResultItemTime(items[j])
			if left.IsZero() || right.IsZero() {
				return false
			}
			return left.After(right)
		})
	}

	// Interleave items across categories round-robin to ensure diversity.
	// The raw feed groups items by category, so naively slicing gives only
	// the first category. Instead, pick up to 2 items per category in
	// round-robin order.
	if data.Query == "" {
		catOrder := []string{}
		catItems := map[string][]formattedNewsItem{}
		for _, a := range items {
			cat := a.Category
			if cat == "" {
				cat = "_"
			}
			if _, ok := catItems[cat]; !ok {
				catOrder = append(catOrder, cat)
			}
			catItems[cat] = append(catItems[cat], a)
		}
		var mixed []formattedNewsItem
		maxPerCat := 3
		for round := 0; round < maxPerCat; round++ {
			for _, cat := range catOrder {
				if round < len(catItems[cat]) {
					mixed = append(mixed, catItems[cat][round])
				}
			}
		}
		items = mixed
	}

	if len(items) > 20 {
		items = items[:20]
	}
	var sb strings.Builder
	if data.Query != "" {
		sb.WriteString(fmt.Sprintf("News results for %q:\n", data.Query))
	} else {
		sb.WriteString("Latest news:\n")
	}
	if freshnessNotice := strings.TrimSpace(data.Freshness.Notice); freshnessNotice != "" {
		sb.WriteString("Freshness caveat: " + freshnessNotice + "\n")
	} else if data.Freshness.Status == "stale" || data.Freshness.Status == "no_dated_results" {
		requestedDate := strings.TrimSpace(data.Freshness.RequestedDate)
		if requestedDate == "" {
			requestedDate = "the requested date"
		}
		if data.Freshness.Status == "no_dated_results" {
			sb.WriteString("Freshness caveat: No dated news results were available for " + requestedDate + "; do not present these results as today's news without that caveat.\n")
		} else {
			freshest := strings.TrimSpace(data.Freshness.FreshestPostedAt)
			if freshest != "" {
				if t, err := time.Parse(time.RFC3339, freshest); err == nil {
					freshest = t.UTC().Format("2006-01-02")
				}
			}
			if freshest == "" {
				sb.WriteString("Freshness caveat: No same-day news results were available for " + requestedDate + "; lead with a freshness caveat before older items.\n")
			} else {
				sb.WriteString("Freshness caveat: No same-day news results were available for " + requestedDate + "; the freshest result is from " + freshest + ", so lead with a freshness caveat before older items.\n")
			}
		}
	}
	for i, a := range items {
		line := fmt.Sprintf("%d. %s", i+1, a.Title)
		if desc := cleanNewsDescription(a.Description); desc != "" {
			line += " — " + desc
		}
		if label := conciseNewsSourceDateLabel(a); label != "" {
			line += " (" + label + ")"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func cleanNewsDescription(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	desc = strings.TrimRight(desc, " \t\n\r")
	for strings.HasSuffix(desc, "...") || strings.HasSuffix(desc, "…") {
		desc = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(desc, "..."), "…"))
	}
	return desc
}

func conciseNewsSourceDateLabel(item formattedNewsItem) string {
	var parts []string
	if source := newsSourceLabel(item.URL); source != "" {
		parts = append(parts, source)
	}
	if date := newsDateLabel(item); date != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, ", ")
}

func newsSourceLabel(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	return host
}

func newsDateLabel(item formattedNewsItem) string {
	if t := newsResultItemTime(item); !t.IsZero() {
		return t.Format("2 Jan 2006")
	}
	return ""
}

type formattedNewsItem struct {
	Title       string
	Description string
	Category    string
	URL         string
	PostedAt    string
	Published   string
}

func newsResultItemTime(item formattedNewsItem) time.Time {
	for _, value := range []string{item.PostedAt, item.Published} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2 Jan 2006 15:04 MST", "2 Jan 2006", "2006-01-02", "Jan 2, 2006"} {
			if t, err := time.Parse(layout, value); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Time{}
}

// formatVideoResult converts a raw JSON video search result into
// human-readable text for the AI synthesis RAG context.
func formatVideoResult(result string) string {
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			Channel string `json:"channel"`
			URL     string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Results) == 0 {
		return "No videos found."
	}
	items := data.Results
	if len(items) > 10 {
		items = items[:10]
	}
	var sb strings.Builder
	sb.WriteString("Video results:\n")
	for i, v := range items {
		line := fmt.Sprintf("%d. %s", i+1, v.Title)
		if v.Channel != "" {
			line += fmt.Sprintf(" (channel: %s)", v.Channel)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// formatReminderResult converts a raw JSON reflection payload into
// human-readable text for the AI synthesis RAG context.
func formatReminderResult(result string) string {
	var data struct {
		Verse   string `json:"verse"`
		Name    string `json:"name"`
		Hadith  string `json:"hadith"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if data.Verse == "" && data.Hadith == "" && data.Message == "" {
		return "No reflection is available right now."
	}
	// Verse, Saying, Name, Reflection — the same four labels the /prayer page
	// and the prayer_reflection tool use.
	var sb strings.Builder
	sb.WriteString("Today's Islamic reflection:\n")
	if data.Verse != "" {
		sb.WriteString(fmt.Sprintf("Verse: %s\n", data.Verse))
	}
	if data.Hadith != "" {
		sb.WriteString(fmt.Sprintf("Saying: %s\n", data.Hadith))
	}
	if data.Name != "" {
		sb.WriteString(fmt.Sprintf("Name: %s\n", data.Name))
	}
	if data.Message != "" {
		sb.WriteString(fmt.Sprintf("Reflection: %s\n", data.Message))
	}
	return sb.String()
}

// formatSearchResult converts a raw search result (which may be an HTML page)
// into human-readable text for the AI synthesis RAG context.
func formatSearchResult(result string) string {
	// Try to parse as JSON first (structured response)
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Type    string `json:"type"`
		} `json:"results"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(result), &data); err == nil && len(data.Results) > 0 {
		var sb strings.Builder
		if data.Query != "" {
			sb.WriteString(fmt.Sprintf("Search results for %q:\n", data.Query))
		} else {
			sb.WriteString("Search results:\n")
		}
		for i, r := range data.Results {
			line := fmt.Sprintf("%d. %s", i+1, r.Title)
			if r.Type != "" {
				line += fmt.Sprintf(" [%s]", r.Type)
			}
			if r.Content != "" {
				snippet := r.Content
				if len(snippet) > 120 {
					snippet = snippet[:120] + "…"
				}
				line += " — " + snippet
			}
			sb.WriteString(line + "\n")
		}
		return sb.String()
	}
	// Fall back: strip HTML tags to extract plain text
	return stripHTMLTags(result)
}

// formatWebFetchResult converts a raw JSON web fetch result into
// human-readable text for the AI synthesis RAG context.
func formatWebFetchResult(result string) string {
	var data struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	var sb strings.Builder
	if data.Title != "" {
		sb.WriteString(fmt.Sprintf("Page: %s\n", data.Title))
	}
	if data.URL != "" {
		sb.WriteString(fmt.Sprintf("URL: %s\n", data.URL))
	}
	sb.WriteString("\n")
	content := data.Content
	// Truncate for AI context — keep it reasonable
	if len(content) > 8000 {
		content = content[:8000] + "\n\n[Content truncated for brevity]"
	}
	sb.WriteString(content)
	return sb.String()
}

// stripHTMLTags removes HTML tags from s and collapses whitespace.
func stripHTMLTags(s string) string {
	var sb strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '<':
			inTag = true
		case s[i] == '>':
			inTag = false
			sb.WriteByte(' ')
		case !inTag:
			sb.WriteByte(s[i])
		}
	}
	// Collapse runs of whitespace
	out := strings.Join(strings.Fields(sb.String()), " ")
	if len(out) > 2000 {
		out = out[:2000] + "…"
	}
	return out
}

// formatPlacesResult converts a raw JSON places result into a human-readable
// text summary suitable for inclusion in the AI synthesis RAG context.
func formatPlacesResult(result string, args map[string]any) string {
	var data struct {
		Results []placeItem `json:"results"`
		Count   int         `json:"count"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Results) == 0 {
		return "No places found."
	}

	q := ""
	near := ""
	if args != nil {
		if v, ok := args["q"]; ok {
			q = fmt.Sprintf("%v", v)
		}
		if v, ok := args["near"]; ok {
			near = fmt.Sprintf("%v", v)
		}
		if near == "" {
			if v, ok := args["address"]; ok {
				near = fmt.Sprintf("%v", v)
			}
		}
	}

	var sb strings.Builder
	header := fmt.Sprintf("Found %d place(s)", len(data.Results))
	if q != "" && near != "" {
		header += fmt.Sprintf(" matching %q near %s", q, near)
	} else if q != "" {
		header += fmt.Sprintf(" matching %q", q)
	} else if near != "" {
		header += fmt.Sprintf(" near %s", near)
	}
	sb.WriteString(header + ":\n")
	for i, p := range data.Results {
		line := fmt.Sprintf("%d. %s", i+1, p.Name)
		if p.Category != "" {
			line += " (" + p.Category + ")"
		}
		if p.Address != "" {
			line += " — " + p.Address
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

type placeItem struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Address  string  `json:"address"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

// formatWalletBalanceResult converts a raw JSON wallet balance result into
// human-readable text for the AI synthesis RAG context.
//
// formatWalletBalanceResult renders what the wallet holds.
//
// The wallet is a key on Base now and the number it returns is USDC. It used to
// be a credit balance, and answers in that shape still format — an older client,
// or a conversation replayed from before the split, should not come back as raw
// JSON. The two are labelled differently on purpose: mistaking one for the other
// is how somebody tries to pay for a tool call with money the meter cannot see.
func formatWalletBalanceResult(result string) string {
	var data struct {
		Credits *int   `json:"credits"`
		Balance *int   `json:"balance"`
		Address string `json:"address"`
		USDC    string `json:"usdc"`
		Network string `json:"network"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}

	var sb strings.Builder
	// Credits, when something answered in the old shape.
	if credits := data.Credits; credits != nil || data.Balance != nil {
		n := 0
		if credits != nil {
			n = *credits
		} else {
			n = *data.Balance
		}
		fmt.Fprintf(&sb, "Credit balance: %d credits ($%d.%02d). Top up at /wallet/topup.\n", n, n/100, n%100)
	}
	switch {
	case data.USDC != "":
		net := data.Network
		if net == "" {
			net = "Base"
		}
		fmt.Fprintf(&sb, "Wallet: %s holds $%s USDC on %s.\n", data.Address, data.USDC, net)
	case data.Address != "":
		fmt.Fprintf(&sb, "Wallet address: %s.\n", data.Address)
	}
	if sb.Len() == 0 {
		if data.Text != "" {
			return data.Text
		}
		return result
	}
	return sb.String()
}

// formatAppsSearchResult converts a raw JSON apps search result into
// human-readable text for the AI synthesis RAG context.
func formatAppsSearchResult(result string) string {
	var apps []struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Tags        string `json:"tags"`
		Installs    int    `json:"installs"`
	}
	if err := json.Unmarshal([]byte(result), &apps); err != nil {
		return result
	}
	if len(apps) == 0 {
		return "No apps found."
	}
	var sb strings.Builder
	sb.WriteString("Apps:\n")
	for i, a := range apps {
		tagInfo := ""
		if a.Tags != "" {
			tagInfo = " [" + a.Tags + "]"
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s) — %s%s %d installs /apps/%s\n",
			i+1, a.Name, a.Slug, a.Description, tagInfo, a.Installs, a.Slug))
	}
	return sb.String()
}

// formatAppsReadResult converts a raw JSON app detail result into
// human-readable text for the AI synthesis RAG context.
func formatAppsReadResult(result string) string {
	var a struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Tags        string `json:"tags"`
		Installs    int    `json:"installs"`
	}
	if err := json.Unmarshal([]byte(result), &a); err != nil {
		return result
	}
	if a.Name == "" {
		return result
	}
	tagLine := ""
	if a.Tags != "" {
		tagLine = fmt.Sprintf("Tags: %s\n", a.Tags)
	}
	// One URL, and it is the app. There was a second line here — "Run:
	// /apps/<slug>/run" — naming a path that 404s: "run" is retired, and an
	// agent repeating a dead link to somebody is worse than saying nothing,
	// because they will click it.
	return fmt.Sprintf("App: %s\nID: %s\nDescription: %s\nAuthor: %s\n%sInstalls: %d\nURL: /apps/%s\n",
		a.Name, a.Slug, a.Description, a.Author, tagLine, a.Installs, a.Slug)
}

// formatAppsBuildResult converts a raw JSON build result into
// human-readable text for the AI synthesis RAG context.
func formatAppsBuildResult(result string) string {
	var data struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return "App HTML generated successfully."
	}
	if data.HTML == "" {
		return result
	}
	snippet := data.HTML
	if len(snippet) > 200 {
		snippet = snippet[:200] + "…"
	}
	return fmt.Sprintf("Generated app HTML (%d bytes). Preview:\n%s", len(data.HTML), snippet)
}

// renderAppsCard renders an HTML card for apps search results.
func renderAppsCard(result string) string {
	var apps []struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Tags        string `json:"tags"`
		Installs    int    `json:"installs"`
	}
	if err := json.Unmarshal([]byte(result), &apps); err != nil {
		return ""
	}
	if len(apps) == 0 {
		return ""
	}
	if len(apps) > 5 {
		apps = apps[:5]
	}
	var b strings.Builder
	b.WriteString(`<div class="card"><h4>📱 Apps</h4>`)
	for _, a := range apps {
		b.WriteString(`<div class="thin-row">`)
		b.WriteString(`<a href="/apps/` + htmlEsc(a.Slug) + `" class="semibold">` + htmlEsc(a.Name) + `</a>`)
		b.WriteString(`<div class="text-xs text-muted">` + htmlEsc(a.Description) + `</div>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`<a href="/apps" class="link d-inline-block mt-2">Browse all apps →</a></div>`)
	return b.String()
}

// htmlEsc escapes a string for safe HTML attribute/text inclusion.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// htmlEsc escapes text for HTML.
//
// It delegates rather than reimplementing, because this package had its own
// version and it escaped one character fewer than the others: & < > and the
// double quote, but not the single quote. Nothing here puts output in a
// single-quoted attribute today, so it was a hazard rather than a hole — and
// the next single-quoted attribute would have made it one, with nothing to say
// the escaper was weaker than it looked.
func htmlEsc(s string) string { return html.EscapeString(s) }
