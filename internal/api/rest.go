package api

// The API a program calls when it is not an agent.
//
// /mcp is the agent door and stays the agent door: JSON-RPC, a session
// handshake, a catalogue to plan over. That is right for something choosing a
// tool and wrong for something that already knows which one it wants. A desktop
// client, a native app, a front end that is not this binary's HTML — none of
// them are planning, they are calling a known method, and making them speak a
// tool-calling protocol to do it is a tax.
//
// This is not a second agent door and must not be documented as one. /api used
// to carry a REST surface with its own auth story and its own price table, and
// it was removed for a good reason: two documented ways in is a decision the
// reader has to make before they can start. Nothing here has its own anything.
// A request is turned into a tool name and handed to ExecuteTool, which is the
// same function /mcp calls — so scope checks, account-only refusals, quota,
// identity binding and pricing are not reimplemented, they are inherited. If a
// permission is wrong here it is wrong at /mcp too, which is the property worth
// having.
//
// The shape was already in the codebase, addressed as though it belonged to
// apps: /apps/{slug}/sdk/service takes {service, method, args}, binds identity
// from the session and refuses account-scoped services. That last part is right
// for a sandboxed app running somebody else's JavaScript and wrong for a signed
// -in client, which is the one thing this does differently — a caller here gets
// what their identity is entitled to, and that is decided per tool by the same
// code as everywhere else.

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// RESTPrefix is the door. Versioned because it is the one surface here that a
// program outside this repository compiles against.
//
// RESTRoot is the same thing without the trailing slash, and both are needed
// rather than one being tidier than the other. internal/server strips a
// trailing slash from every request path before routing, so a subtree pattern
// registered only as "/api/v1/" is reached at "/api/v1" — which the mux answers
// by redirecting to "/api/v1/", which is stripped again. An infinite redirect
// on the catalogue, and no test would have caught it because every path a test
// asks about has something after the prefix. Every other subtree route in this
// codebase registers both forms for the same reason.
const (
	RESTPrefix = "/api/v1/"
	RESTRoot   = "/api/v1"
)

// authRefusal is what this codebase says when there is no caller. Both
// spellings of it — see restStatus.
const authRefusal = "authentication required"

// RESTToolName turns a request path into the tool it names, or "" if the path
// is not a call — which includes the catalogue at the root.
// /api/v1/news/list is news_list.
//
// Exported because the door middleware in internal/server has to know which
// tool a request is for before the handler runs — the payment challenge and the
// authentication challenge both happen upstream of the mux.
func RESTToolName(path string) string {
	// The prefix has to be checked before it is trimmed. TrimPrefix on a path
	// that does not carry it returns the path unchanged, so "/news/list" would
	// split into two parts and resolve to news_list — the door answering for a
	// page it has nothing to do with.
	if path != RESTRoot && !strings.HasPrefix(path, RESTPrefix) {
		return ""
	}
	rest := strings.Trim(strings.TrimPrefix(path, RESTRoot), "/")
	if rest == "" {
		return ""
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return strings.ToLower(parts[0]) + "_" + strings.ToLower(parts[1])
}

// ToolDispatch reports whether a path is one a tool call arrives at.
//
// There are two and there should not be a third without a reason: /mcp for
// something choosing a tool, and /api/v1/ for something that already knows
// which one it wants. Everything upstream of the mux — verifying a wallet
// signature, challenging for authentication, taking payment — has to happen for
// both, and asking here is what stops that list being written out twice with
// one of the copies missing an entry.
func ToolDispatch(path string) bool {
	return path == "/mcp" || path == RESTRoot || strings.HasPrefix(path, RESTPrefix)
}

// RequestTool returns the tool an inbound request names, for either door.
//
// body is the request body, already read and put back by the caller; it is what
// the MCP door carries its tool name in and is ignored by the REST one, whose
// name is in the path.
func RequestTool(path string, body []byte) string {
	if path == "/mcp" {
		return mcpToolName(body)
	}
	return RESTToolName(path)
}

// RESTHandler serves /api/v1/.
//
//	GET  /api/v1/                 the catalogue
//	GET  /api/v1/news/list        arguments from the query string
//	POST /api/v1/news/list        arguments from a JSON body
//
// GET and POST both work and mean the same thing. A REST API where reads are
// GETs is what every client library and every curl in a bug report expects, and
// refusing that to keep one code path would be a purity nobody asked for. What
// is not offered is a GET that changes something: a method the catalogue marks
// as writing needs a POST, so a link, a prefetch or a crawler cannot fire one.
// That is Writes on the Endpoint, and it is a wider set than Destructive —
// notes_add is safe to hand an agent and is still not something a URL should
// do.
func RESTHandler(w http.ResponseWriter, r *http.Request) {
	if strings.Trim(strings.TrimPrefix(r.URL.Path, RESTRoot), "/") == "" {
		restCatalogue(w, r)
		return
	}

	name := RESTToolName(r.URL.Path)
	if name == "" {
		app.RespondError(w, http.StatusNotFound,
			"Call a service method: "+RESTPrefix+"<service>/<method>")
		return
	}

	var args map[string]any
	switch r.Method {
	case http.MethodGet:
		// Changes, not Destructive. The two were one flag, so a method that
		// wrote but was safe for the model to hold — notes_add, docs_write,
		// files_put — went out as a GET, and a URL that changes your data when
		// something follows it is the oldest mistake on the web.
		if service.ChangingTool(name) {
			w.Header().Set("Allow", "POST")
			app.RespondError(w, http.StatusMethodNotAllowed,
				name+" changes something and cannot be a GET")
			return
		}
		args = queryArgs(r, name)
	case http.MethodPost:
		// A cookie is not a credential a program should be relying on here, and
		// a browser sends it whoever asked. Anything authenticating by header —
		// a token, a wallet signature, a payment — is not forgeable cross-site
		// and is let through; a call resting on the session cookie has to prove
		// it came from a page this instance served.
		//
		// StrictCSRF rather than ValidCSRF, which allows a request that omits
		// the token entirely for the sake of pages already in the wild. This
		// door has no pages in the wild.
		if !headerCredential(r) && !auth.StrictCSRF(r) {
			app.RespondError(w, http.StatusForbidden,
				"A cookie-authenticated call needs an X-CSRF-Token header. "+
					"Programs should send Authorization: Bearer <token> instead.")
			return
		}

		args = map[string]any{}
		// An empty body is a call with no arguments, not a bad request.
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
		if err := dec.Decode(&args); err != nil && err.Error() != "EOF" {
			app.RespondError(w, http.StatusBadRequest, "Body must be a JSON object of arguments")
			return
		}
	default:
		// JSON, not app.MethodNotAllowed, which renders an HTML page. A client
		// here parses what it is given, and a page of markup where an error
		// object was expected is a parse failure rather than a message.
		w.Header().Set("Allow", "GET, POST")
		app.RespondError(w, http.StatusMethodNotAllowed, r.Method+" is not allowed here")
		return
	}

	text, failed, err := ExecuteTool(r, name, args)
	if err != nil {
		app.RespondError(w, restStatus(text, err), err.Error())
		return
	}
	if failed {
		app.RespondError(w, restStatus(text, nil), text)
		return
	}

	app.RespondJSON(w, map[string]any{"result": text})
}

// headerCredential reports whether the caller identified itself with something
// a browser does not attach on its own.
//
// That is the whole CSRF question, asked the useful way round. A cookie rides
// along with a request whoever caused it; a header has to be put there by the
// program making the call, so a page on another origin cannot supply one.
func headerCredential(r *http.Request) bool {
	return r.Header.Get("Authorization") != "" ||
		r.Header.Get(TokenHeader) != "" ||
		WalletSigner(r) != "" ||
		WalletPayer(r) != ""
}

// queryArgs reads a GET's arguments, typed the way the tool declared them.
//
// A query string is all strings, and the handlers behind these tools take
// numbers and booleans, so lat=51.5 arrived as "51.5" and the dispatcher
// answered "cannot unmarshal string into Go struct field". Correct and useless:
// the caller wrote the only thing a URL can carry.
//
// The types are on the tool already — they are what tools/list publishes as its
// schema — so this reads them rather than guessing from the shape of the value.
// Guessing would turn a postcode, a version or an id that happens to be all
// digits into a number. Anything the tool did not declare, or declared as a
// string, is passed through untouched, and a value that does not parse as its
// declared type is passed through too so the handler can say why.
func queryArgs(r *http.Request, tool string) map[string]any {
	want := map[string]string{}
	if t, ok := ToolByName(tool); ok {
		for _, p := range t.Params {
			want[strings.ToLower(p.Name)] = p.Type
		}
	}

	args := map[string]any{}
	for k, v := range r.URL.Query() {
		if len(v) == 0 {
			continue
		}
		args[k] = typed(v[0], want[strings.ToLower(k)])
	}
	return args
}

// typed converts one query value to its declared JSON type.
func typed(raw, want string) any {
	switch want {
	case "number":
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			return f
		}
	case "integer":
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	case "boolean":
		if b, err := strconv.ParseBool(raw); err == nil {
			return b
		}
	}
	return raw
}

// restStatus turns a refusal into the status code a client can act on.
//
// The door middleware answers the refusals it can see coming — a tool that
// needs an account, a tool that needs paying for — with 401 or 402 and the
// headers that say what to do next. Everything else is decided inside
// ExecuteTool, which reports in prose because its other caller is a model, and
// arrives here as a bare error. Mapping all of that to 400 tells a client that
// its arguments were wrong when what actually happened was that it was not
// signed in, which is the difference between retrying and re-authenticating.
//
// Matched on whole phrases this codebase writes for exactly this, not on
// substrings: "authentication required" is what internal/auth returns and what
// ExecuteTool passes on, and a refusal that merely mentions authentication
// somewhere in a longer sentence is a service explaining itself, which is an
// ordinary 400.
//
// Two spellings, because there are two places it comes from and they differ in
// one capital letter. ExecuteTool writes "Authentication required" when it
// turns a caller away before dispatch; a spec-derived tool dispatches, and the
// service refuses with auth's own lowercase error. blog_delete answered 400
// for that second reason until this matched both.
func restStatus(text string, err error) int {
	msg := text
	if err != nil {
		msg = err.Error()
	}
	switch {
	case strings.HasPrefix(msg, "unknown tool"):
		return http.StatusNotFound
	case strings.EqualFold(text, authRefusal), strings.EqualFold(msg, authRefusal):
		return http.StatusUnauthorized
	case text == "This tool requires an account":
		// Not 401. The caller proved who they are — with a wallet — and that
		// identity is not enough for this tool. Signing in again as the same
		// wallet would change nothing, and a client told to retry would loop.
		return http.StatusForbidden
	default:
		return http.StatusBadRequest
	}
}

// restCatalogue lists what can be called, derived from the same specs the tool
// list is derived from. A client that wants to know what exists asks here
// rather than reading a document that has to be kept in step.
func restCatalogue(w http.ResponseWriter, r *http.Request) {
	type method struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		Doc         string `json:"doc,omitempty"`
		Cost        string `json:"cost,omitempty"`
		Destructive bool   `json:"destructive,omitempty"`
		NeedsAuth   bool   `json:"needs_auth"`
	}
	type svc struct {
		Service     string   `json:"service"`
		Description string   `json:"description,omitempty"`
		Scoped      bool     `json:"scoped"`
		Methods     []method `json:"methods"`
	}

	var out []svc
	for _, sp := range service.Specs() {
		s := svc{Service: sp.Name, Description: sp.Description, Scoped: sp.Scoped}
		for name, ep := range sp.Endpoints {
			tool := sp.Name + "_" + strings.ToLower(name)
			s.Methods = append(s.Methods, method{
				Method:      name,
				Path:        RESTPrefix + sp.Name + "/" + strings.ToLower(name),
				Doc:         ep.Doc,
				Cost:        ep.Cost,
				Destructive: ep.Destructive,
				NeedsAuth:   ToolNeedsAuth(tool),
			})
		}
		sort.Slice(s.Methods, func(i, j int) bool { return s.Methods[i].Method < s.Methods[j].Method })
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })

	app.RespondJSON(w, map[string]any{"services": out})
}
