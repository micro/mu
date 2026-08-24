package agent

// Talking to an agent from a program.
//
// There was no way to. The Connect page offered an "Endpoint", and it was
// /mcp — which is the *tools* this agent may use, for something else's agent to
// call. Point Claude at it and Claude uses your services under its own
// instruction; your agent's prompt, its memory and its name are not involved.
// Over MCP an agent is a scoped API key, and the page implying otherwise was
// the honest complaint that started this.
//
// The other door was no better an answer: email reaches exactly the right agent
// but is asynchronous, which is right for a person and wrong for a program
// waiting on a reply. There was a third, /a2a, running a generic account-less
// orchestration that was nobody's agent — it has been deleted rather than
// explained.
//
// So: POST /agent/<name>, with the account's token. Same agent, same standing
// instruction, same scope, same conversation record as the chat page at that
// address — because it is the same call. What differs is that a program made it.
//
// # Why it is not a service
//
// A service answers a question about state and tools are derived from it. This
// consumes tools, so it declares no Spec and appears in no catalogue — the rule
// in CLAUDE.md, which is also why agent_ask was removed as a tool. It is a door
// onto the agent, next to /mcp, and it lives here because the agent is what it
// serves.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// AskRequest is what a program sends.
//
// Text is the question. Thread continues a conversation — pass back what the
// last answer returned and the agent has the history, which is the whole
// difference between an agent and a completion endpoint. Leave it out and one
// is opened.
type apiAsk struct {
	Text string `json:"text"`
	// Thread is a conversation id from a previous answer.
	Thread string `json:"thread,omitempty"`
}

// apiAnswer is what comes back.
//
// Thread is returned always, including on the first call, because a caller that
// wants a second turn needs it and has no other way to learn it. Flow is the
// workflow record — how the answer was produced — for anybody who wants to look
// at what it did.
type apiAnswer struct {
	Text   string `json:"text"`
	Thread string `json:"thread,omitempty"`
	Agent  string `json:"agent,omitempty"`
	Flow   string `json:"flow,omitempty"`
}

// askLimit bounds one question. Long enough for a paragraph with a document
// quoted in it; short enough that a pasted book is not a prompt.
const askLimit = 8000

// APIHandler serves POST /agent/<name>.
//
// Registered alongside the page at the same path, because they are the same
// place: GET reads the conversation, POST asks a question. A second path —
// /api/agent/<name> — would be a second name for one thing, and the address of
// an agent is the address of an agent.
func APIHandler(w http.ResponseWriter, r *http.Request) {
	// Bearer token or session. RequireSession takes both, which is what makes
	// the same endpoint work from a program and from the page's own script.
	sess, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		app.Unauthorized(w, r)
		return
	}
	accountID := sess.Account

	slug := strings.TrimPrefix(r.URL.Path, "/agent/")
	if slug == "" || strings.Contains(slug, "/") {
		app.NotFound(w, r, "no agent named")
		return
	}
	// The same lookup the page does, so /agent/research and POST /agent/research
	// cannot disagree about which agent that is.
	agentID, ok := agentSlugTarget(accountID, slug)
	if !ok {
		app.NotFound(w, r, "no agent called "+slug)
		return
	}

	var req apiAsk
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, askLimit*4)).Decode(&req); err != nil {
		app.BadRequest(w, r, "send JSON: {\"text\": \"...\"}")
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		app.BadRequest(w, r, "nothing to ask")
		return
	}
	if len(req.Text) > askLimit {
		req.Text = req.Text[:askLimit]
	}
	// A conversation belonging to somebody else is not a conversation. Checked
	// before the model is asked, so a forged id is a refusal rather than an
	// answer filed on a stranger's thread.
	if req.Thread != "" && thread.Get(accountID, req.Thread) == nil {
		app.NotFound(w, r, "no conversation here with that id")
		return
	}

	// On, not Thread. They are different fields for different things and this
	// used the wrong one, six lines below validating it as the other.
	//
	// AskRequest.Thread is the *client's own* key for a conversation — mail's
	// chain key, a channel id — which Ask resolves with thread.Open. On is a
	// conversation in the record the caller is already holding, by id, which
	// Ask resolves with thread.Get. What this door hands back is an id, and the
	// check above looks it up with thread.Get, so an id is what comes in.
	//
	// Passed as Thread it was searched for as a key, matched nothing, and
	// opened a *new* conversation whose key happened to be the previous
	// conversation's id. So the second call came back with a different thread
	// id and an agent that had never heard of the first question — the exact
	// failure the field exists to prevent, on the exact endpoint whose whole
	// claim is that passing the id back continues the conversation.
	res, err := Ask(AskRequest{
		Account: accountID,
		Client:  thread.WebClient,
		On:      req.Thread,
		Text:    req.Text,
		Agent:   agentID,
		Trigger: "api",
	})
	if err != nil {
		app.ServerError(w, r, "that one did not work")
		return
	}

	app.RespondJSON(w, apiAnswer{
		Text:   res.Text,
		Thread: res.Thread,
		Agent:  SlugFor(accountID, agentID),
		Flow:   res.Flow,
	})
}

// credits says a count so the refusal reads as a sentence.
func credits(n int) string {
	if n == 1 {
		return "1 credit"
	}
	return strconv.Itoa(n) + " credits"
}
