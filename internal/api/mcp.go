package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"mu/internal/service"

	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/usage"
)

// MCP protocol version
const MCPVersion = "2025-03-26"

// JSON-RPC types
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP types
type mcpInitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	ClientInfo      mcpClientInfo  `json:"clientInfo"`
	Capabilities    map[string]any `json:"capabilities"`
}

type mcpClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpInitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	ServerInfo      mcpServerInfo   `json:"serverInfo"`
	Capabilities    mcpCapabilities `json:"capabilities"`
}

type mcpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpCapabilities struct {
	Tools *mcpToolCapability `json:"tools,omitempty"`
}

type mcpToolCapability struct{}

type mcpToolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema mcpInputSchema `json:"inputSchema"`
}

type mcpInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]mcpProperty `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

type mcpProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Tool defines an MCP tool with its HTTP mapping
type Tool struct {
	Name        string      `json:"name"`
	Aliases     []string    `json:"-"` // legacy names that still resolve to this tool (not shown in listings)
	Description string      `json:"description"`
	Title       string      `json:"title,omitempty"` // display title, if a caller sets one
	Icon        string      `json:"icon,omitempty"`
	Method      string      `json:"method,omitempty"`
	Path        string      `json:"path,omitempty"`
	Params      []ToolParam `json:"params,omitempty"`
	WalletOp    string      `json:"walletOp,omitempty"` // Wallet operation for credit gating (empty = included)
	// AccountOnly refuses a caller whose only identity is a paid wallet. Paying
	// proves funds, not accountability — mail_send is the case that matters,
	// because an anonymous funded wallet sending from this domain spends its
	// reputation and cannot be held to it.
	AccountOnly bool `json:"-"`
	// RESTOnly marks an HTTP endpoint that is not an agent tool. REST paths are
	// resource-shaped (/news, /mail) while tools are service_method
	// (news_list, mail_inbox); the same capability legitimately appears in both
	// systems under different names, but it must not appear twice in the tool
	// list under two names. See restTools and mcpTools.
	RESTOnly   bool                                         `json:"-"`
	Handle     func(map[string]any) (string, error)         `json:"-"` // Optional direct handler (bypasses HTTP dispatch)
	HandleAuth func(map[string]any, string) (string, error) `json:"-"` // Like Handle but receives the account ID
	// OptionalAuth runs HandleAuth with an empty account rather than refusing
	// when there is no caller. For a tool that answers anyone but answers a
	// signed-in caller with more — index_search returns public content to a
	// guest and the caller's own on top of it. Without this the choice was
	// between refusing guests and never learning who is asking, and refusing
	// guests is what shipped, against the service's own declared design.
	OptionalAuth bool `json:"-"`
}

// QuotaCheck is called before executing a metered tool.
// It receives the HTTP request (for auth) and the wallet operation string.
// Returns (canProceed, creditCost, error).
// Set by main.go to wire in auth + wallet packages without import cycles.
var QuotaCheck func(r *http.Request, op string) (bool, int, error)

// GuestNewsSearch handles agent-initiated guest news_search calls without
// routing through the authenticated HTML/API search endpoint. It is wired by
// main.go to the news package so guest core-loop prompts can use the same live
// feed-backed provider path as signed-in users while MCP/REST quota gates stay
// unchanged.
var GuestNewsSearch func(query string) (string, error)

// ToolParam defines a parameter for an MCP tool
type ToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// MCPWalletOp parses a JSON-RPC MCP request body and returns the wallet
// operation for a tools/call the caller can unlock by paying, or "" if the call
// is free, cannot be unlocked by paying, or is not a tools/call. Lets the HTTP
// layer gate x402 payments before dispatch, where auth and wallet are in scope.
//
// An account-only tool returns "" however it is priced, because a payment
// cannot buy the thing standing in the way. mail_send is the case: it is
// AccountOnly on purpose — a funded wallet is not accountable for this sending
// domain — and it is charged for external delivery, so the gate offered an
// anonymous caller a 402, took USDC on Base for it, and then refused the call
// for having no account. Money for nothing, and nowhere to complain, since the
// payer has no account to complain from.
//
// The charge still happens: service/mail checks and consumes external_email
// itself once it knows who is sending. What is dropped is only the offer to
// sell entry to somebody who will be turned away at the door.
func MCPWalletOp(body []byte) string {
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Method != "tools/call" {
		return ""
	}
	for i := range tools {
		if toolMatches(tools[i], req.Params.Name) {
			if policyOf(tools[i]).NeedsAccount {
				return ""
			}
			return tools[i].WalletOp
		}
	}
	return ""
}

// MCPToolNeedsAuth reports whether a tools/call in this JSON-RPC body names a
// tool that requires an authenticated caller.
//
// The HTTP layer needs this to answer an unauthenticated call with a 401 and a
// WWW-Authenticate pointing at the resource metadata, which is how an MCP
// client discovers that it should start an OAuth flow. Without the challenge
// the discovery documents are never fetched, so the standard way of connecting
// silently does not work.
//
// Public tools stay anonymous: a challenge on those would make news and weather
// unreachable without an account, which is the opposite of the point.
func MCPToolNeedsAuth(body []byte) bool {
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Method != "tools/call" {
		return false
	}
	for i := range tools {
		if !toolMatches(tools[i], req.Params.Name) {
			continue
		}
		if tools[i].OptionalAuth {
			return false // answers a guest, so must not send them to sign in
		}
		if tools[i].HandleAuth != nil || tools[i].AccountOnly {
			return true
		}
		// Path-backed tools authenticate inside their own HTTP handler, so
		// neither flag is set on them — mail_inbox is one. Their service knows,
		// though: a scoped service is closed to callers with no account, and
		// that is declared once in the Spec.
		return service.AccountScoped(serviceOf(tools[i].Name))
	}
	return false
}

// toolMatches reports whether name is the tool's canonical name or one of its
// legacy aliases. Aliases let a renamed tool keep resolving old callers (agent
// shortcuts, existing integrations) without appearing in listings.
func toolMatches(t Tool, name string) bool {
	if t.Name == name {
		return true
	}
	for _, a := range t.Aliases {
		if a == name {
			return true
		}
	}
	return false
}

// RegisterTool adds a tool to the MCP server.
// HasTool reports whether a tool of that name, or one of its aliases, is
// registered.
func HasTool(name string) bool {
	for i := range tools {
		if toolMatches(tools[i], name) {
			return true
		}
	}
	return false
}

// Tools is the registry as it stands: every tool a client can call, in
// registration order. The handlers come with them, so this is the registry
// itself and not a description of it — treat the result as read-only.
func Tools() []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	return out
}

func RegisterTool(t Tool) {
	tools = append(tools, t)
}

// RegisterToolWithAuth adds a tool that receives the authenticated account ID.
func RegisterToolWithAuth(t Tool, handler func(map[string]any, string) (string, error)) {
	t.HandleAuth = handler
	tools = append(tools, t)
}

// ToolDescriptions returns a simple "- name: description" list of all tools,
// suitable for agent planning prompts.
func ToolDescriptions() string {
	var sb strings.Builder
	for _, t := range tools {
		if t.Name == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("- %s: %s", t.Name, t.Description))
		if len(t.Params) > 0 {
			sb.WriteString(" (args: {")
			for i, p := range t.Params {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf(`"%s":"%s"`, p.Name, p.Type))
			}
			sb.WriteString("})")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// tools is the list of MCP tools derived from API endpoints.
//
// Session establishment is deliberately absent. Signing up, logging in and
// reading back who you are are not capabilities a caller can be granted — they
// are how a caller comes to exist, and they belong to the HTTP boundary (the
// login form, /session, a Personal Access Token) and to the CLI. An agent never
// walks that path: it authenticates by holding a token a human issued, or by
// paying per request over x402, where there is no account to sign up for.
var tools = []Tool{
	// REST-only: the resource endpoints behind the /news and /search pages. The
	// agent reaches the same capabilities as news_list and web_search, so these
	// are not tools — listing them would be the same thing twice under two names.
	{
		Name:        "news",
		RESTOnly:    true,
		Description: "Read the latest news feed",
		Method:      "GET",
		Path:        "/news",
	},
	{
		Name:        "search",
		RESTOnly:    true,
		Description: "Search the web",
		Method:      "GET",
		Path:        "/search",
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
		},
	},
	// blog_list, social, video, weather_forecast and markets are registered in
	// main.go as AI-first tools (clean Go handlers returning model-ready text),
	// not as page-backed entries here.
	{
		Name:        "blog_read",
		Description: "Read one blog post in full, by id or by title. Use after blog_list or index_search has found a candidate and the summary is not enough. Returns the whole body, the author and when it was published.",
		Method:      "GET",
		Path:        "/blog/post",
		Params: []ToolParam{
			{Name: "id", Type: "string", Description: "The post's id, as given by blog_list"},
			{Name: "title", Type: "string", Description: "The post's title, or enough of it to be unambiguous — use this when you have a name rather than an id"},
		},
	},
	{
		Name:        "blog_create",
		Description: "Publish a post to the caller's blog. Use for anything meant to be read later by other people — notes, write-ups, announcements. Returns the post and its URL. For a private note to yourself, prefer files or memory.",
		Method:      "POST",
		Path:        "/blog/post",
		WalletOp:    quota.OpBlogCreate,
		Params: []ToolParam{
			{Name: "title", Type: "string", Description: "Post title", Required: false},
			{Name: "content", Type: "string", Description: "Post content (minimum 50 characters)", Required: true},
		},
	},
	{
		Name:        "blog_update",
		Description: "Update an existing blog post (author only)",
		Method:      "PATCH",
		Path:        "/blog/post",
		Params: []ToolParam{
			{Name: "id", Type: "string", Description: "The blog post ID to update", Required: true},
			{Name: "title", Type: "string", Description: "New post title", Required: false},
			{Name: "content", Type: "string", Description: "New post content (minimum 50 characters)", Required: false},
		},
	},
	{
		Name:        "blog_delete",
		Description: "Delete one of the caller's own blog posts, by id or title. Refuses posts written by anyone else. Irreversible, so confirm with the user first.",
		Method:      "DELETE",
		Path:        "/blog/post",
		Params: []ToolParam{
			{Name: "id", Type: "string", Description: "The post's id, as given by blog_list"},
			{Name: "title", Type: "string", Description: "The post's title, or enough of it to be unambiguous. An ambiguous title is refused rather than guessed — deleting the wrong post is not recoverable"},
		},
	},
	{
		Name:        "social_search",
		Description: "Search public posts on this instance by keyword. Returns matching posts with their author and time. This is the instance's own social feed, not " + "the wider internet — use web_search for that.",
		Method:      "POST",
		Path:        "/social",
		WalletOp:    quota.OpSocialSearch,
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Search query for social posts", Required: true},
		},
	},
	{
		Name:        "video_search",
		Description: "Search videos from the channels this instance curates, by keyword. Returns titles, channels and links. A curated set rather than all of YouTube, so a miss means it is not followed here, not that it does not exist. Needs an account.",
		Method:      "POST",
		Path:        "/video",
		WalletOp:    quota.OpVideoSearch,
		// Priced at zero but not open: the YouTube quota is 10,000 units a day
		// across everyone and a search costs 100, so this is rationed by
		// videoSearchLimit per account. There is nothing to ration an
		// anonymous caller by. Saying so here refuses them at the MCP layer
		// with a 401 pointing at sign-in, rather than letting them through to
		// a handler refusal — or, before this, offering them a payment that
		// would not have unlocked anything.
		AccountOnly: true,
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Video search query", Required: true},
		},
	},
	{
		Name:        "mail_send",
		AccountOnly: true, // a funded wallet is not accountable for the domain
		Description: "Send an email from the caller's own address on this instance. Takes a recipient address, a subject and a body; resolve a name to an address with contacts_find first. Requires an account, and the mail really is delivered " + "— there is no draft state to undo from.",
		Method:      "POST",
		Path:        "/mail",
		WalletOp:    quota.OpExternalEmail,
		Params: []ToolParam{
			{Name: "to", Type: "string", Description: "Recipient username or email", Required: true},
			{Name: "subject", Type: "string", Description: "Message subject", Required: true},
			{Name: "body", Type: "string", Description: "Message body", Required: true},
		},
	},
}

// ExecuteToolAs calls a tool on behalf of a user account (no HTTP request needed).
// Creates a temporary session for auth. Used by background agents.
func ExecuteToolAs(accountID, name string, args map[string]any) (string, bool, error) {
	if name == "news_search" && GuestNewsSearch != nil {
		query := strings.TrimSpace(fmt.Sprintf("%v", args["query"]))
		if query == "" {
			return "query required", true, fmt.Errorf("query required")
		}
		text, err := GuestNewsSearch(query)
		return text, err != nil, err
	}

	sess, err := auth.CreateSession(accountID)
	if err != nil {
		return "", true, fmt.Errorf("failed to create session: %v", err)
	}

	req, _ := http.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return ExecuteTool(req, name, args)
}

// walletIdentityPrefix namespaces a wallet address used as an account id.
// Account ids are usernames, which cannot contain a colon, so a wallet identity
// can never collide with one — and the prefix means any caller of
// AccountFrom can tell how the caller authenticated.
const walletIdentityPrefix = "x402:"

// IsWalletIdentity reports whether an account id is a paid wallet rather than a
// registered account.
func IsWalletIdentity(id string) bool {
	return strings.HasPrefix(id, walletIdentityPrefix)
}

// callerIdentity resolves who is calling: the signed-in account, or the wallet
// that paid for this request.
//
// The wallet address comes from a settled payment, so it is as authenticated as
// a session — only the key holder could have produced it. Before this, a paying
// agent was told "Authentication required" after it had already paid, which
// made every account-scoped tool unreachable to exactly the callers the MCP
// endpoint is for.
func callerIdentity(r *http.Request) (string, error) {
	if _, acc, err := auth.RequireSession(r); err == nil {
		return acc.ID, nil
	}
	if payer := WalletPayer(r); payer != "" {
		return walletIdentityPrefix + payer, nil
	}
	return "", fmt.Errorf("authentication required")
}

// WalletPayer is set by main.go to billing.PayerFrom, which reads the settled
// payment out of the request context. Wired there to keep this package free of
// a wallet import.
var WalletPayer = func(r *http.Request) string { return "" }

// ExecuteTool calls a registered MCP tool with the given name and arguments,
// forwarding authentication from r. It does NOT check wallet quota — the caller
// is responsible for quota management.
// Returns the tool output text, whether the response is an error, and any Go error.
// queryAliases are the names a search term has been called. The catalogue used
// both: news_search, index_search, social_search and video_search took "query"
// while web_search, apps_search, images_search, places_search and quran_search
// took "q". An agent that learned one from the first tool it called got "No
// query provided." from the next — which is not a thing a caller can debug from
// the outside, since both tools advertise a correct schema for themselves.
//
// The schemas now all say "query". This accepts the old name so anything
// already calling with "q" — the CLI, a saved client config — keeps working.
var queryAliases = []string{"query", "q"}

// QueryArg reads a search term under either name, for handlers that receive
// args directly rather than through ExecuteTool.
func QueryArg(args map[string]any) string {
	for _, k := range queryAliases {
		if v, ok := args[k]; ok {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" {
				return s
			}
		}
	}
	return ""
}

// normaliseArgs fills a declared parameter from an equivalent name the caller
// may have used. It never overwrites a value the caller actually supplied.
func normaliseArgs(tool Tool, args map[string]any) map[string]any {
	if args == nil {
		return args
	}
	for _, p := range tool.Params {
		if !contains(queryAliases, p.Name) {
			continue
		}
		if v, ok := args[p.Name]; ok && fmt.Sprintf("%v", v) != "" {
			continue
		}
		for _, alt := range queryAliases {
			if alt == p.Name {
				continue
			}
			if v, ok := args[alt]; ok && fmt.Sprintf("%v", v) != "" {
				args[p.Name] = v
				break
			}
		}
	}
	return args
}

// missingRequired names the first required parameter the caller left out.
//
// Without this a tool handler reads an empty string and returns prose — "No
// query provided." — as a successful result, so a caller cannot tell a bad call
// from an empty result set. A missing parameter is an error and must arrive as
// one.
func missingRequired(tool Tool, args map[string]any) string {
	for _, p := range tool.Params {
		if !p.Required {
			continue
		}
		v, ok := args[p.Name]
		if !ok || strings.TrimSpace(fmt.Sprintf("%v", v)) == "" {
			return p.Name
		}
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// toolSurface tells an operator where a tool call came from: an agent
// connected over MCP, or Mu's own agent running a tool on someone's behalf.
// Both land here, and they are different kinds of load.
func toolSurface(r *http.Request) string {
	if r != nil && r.URL != nil && r.URL.Path == "/mcp" {
		return "mcp"
	}
	return "agent"
}

// toolCaller is who to count the call against, empty for an unauthenticated
// one. A failure to identify is not an error here — it means guest.
func toolCaller(r *http.Request) string {
	if r == nil {
		return ""
	}
	caller, err := callerIdentity(r)
	if err != nil {
		return ""
	}
	return caller
}

func ExecuteTool(r *http.Request, name string, args map[string]any) (string, bool, error) {
	var tool *Tool
	for i := range tools {
		if toolMatches(tools[i], name) {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		return "", true, fmt.Errorf("unknown tool: %s", name)
	}

	// A scoped token reaches only the services it names.
	//
	// This is what makes an agent an agent rather than a copy of you: you hand
	// a program a credential, and the credential is smaller than your account.
	// Without the check here the scope would be a label on a settings page and
	// nothing else, since every branch below dispatches on the account alone.
	//
	// First, before the usage counter and before the argument check. A caller
	// who may not use a tool should learn that and nothing else — otherwise the
	// refusal doubles as a schema oracle ("db_list requires collection"), and a
	// call that never happened is counted as if it had.
	if err := checkTokenScope(r, tool.Name); err != nil {
		return err.Error(), true, err
	}

	// Count the call by tool name. Every MCP request is a POST to /mcp, so the
	// HTTP layer sees one endpoint for the whole protocol and can say nothing
	// about which tool is busy. Recorded under the canonical name, so an alias
	// does not look like a separate tool. See internal/usage.
	usage.Record(toolSurface(r), tool.Name, toolCaller(r))

	args = normaliseArgs(*tool, args)
	if missing := missingRequired(*tool, args); missing != "" {
		return fmt.Sprintf("%s is required", missing), true,
			fmt.Errorf("%s requires %s", tool.Name, missing)
	}

	if name == "news_search" && GuestNewsSearch != nil {
		if _, _, err := auth.RequireSession(r); err != nil {
			query := strings.TrimSpace(fmt.Sprintf("%v", args["query"]))
			if query == "" {
				return "query required", true, fmt.Errorf("query required")
			}
			text, err := GuestNewsSearch(query)
			return text, err != nil, err
		}
	}

	// Refuse a paid wallet on an account-only tool before any dispatch. This has
	// to sit above the branches: mail_send is a path-backed tool, so a guard
	// inside the HandleAuth branch would never have run for it — which is the
	// one tool the guard exists for.
	if tool.AccountOnly {
		if caller, err := callerIdentity(r); err == nil && IsWalletIdentity(caller) {
			return "This tool requires an account", true,
				fmt.Errorf("%s requires an account, not a paid wallet", tool.Name)
		}
	}

	if tool.HandleAuth != nil {
		// Auth-bound tools must never run without a server-validated identity.
		// There are two: a session, and a wallet that has paid for this request.
		// Both are server-validated — a session by the cookie or token, a wallet
		// by the facilitator having verified the signature and moved the funds.
		// Neither is ever taken from an argument or an unauthenticated header.
		caller, err := callerIdentity(r)
		if err != nil {
			if !tool.OptionalAuth {
				return "Authentication required", true, err
			}
			caller = "" // answer as a guest
		}
		text, err := tool.HandleAuth(args, caller)
		return text, err != nil, err
	}

	if tool.Handle != nil {
		text, err := tool.Handle(args)
		return text, err != nil, err
	}

	path := tool.Path
	var bodyReader io.Reader

	if tool.Method == "GET" {
		query := url.Values{}
		for k, v := range args {
			query.Set(k, fmt.Sprintf("%v", v))
		}
		if len(query) > 0 {
			path += "?" + query.Encode()
		}
	} else {
		bodyJSON, _ := json.Marshal(args)
		bodyReader = strings.NewReader(string(bodyJSON))
	}

	internalReq, err := http.NewRequest(tool.Method, path, bodyReader)
	if err != nil {
		return "", true, fmt.Errorf("failed to create request: %v", err)
	}

	internalReq.Header.Set("Accept", "application/json")
	internalReq.Header.Set("Content-Type", "application/json")

	if c, err := r.Cookie("session"); err == nil {
		internalReq.AddCookie(c)
	}
	if auth := r.Header.Get("Authorization"); auth != "" {
		internalReq.Header.Set("Authorization", auth)
	}
	if token := r.Header.Get(TokenHeader); token != "" {
		internalReq.Header.Set(TokenHeader, token)
	}

	recorder := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(recorder, internalReq)

	isError := recorder.Code >= 400
	return recorder.Body.String(), isError, nil
}

// checkTokenScope refuses a tool outside the presented token's scope.
//
// Nil token means the caller authenticated some other way — a cookie session,
// or a settled payment — and is not confined. Unscoped tokens, which is every
// token issued before scopes existed, reach everything exactly as before.
//
// A tool with no service behind it (the platform verbs) is only reachable by an
// unscoped token. That is the conservative reading of a whitelist: somebody who
// said "this agent may use news and mail" did not say "and whatever else is not
// a service".
func checkTokenScope(r *http.Request, toolName string) error {
	if r == nil {
		return nil
	}
	tok := auth.TokenFromRequest(r)
	if tok == nil || !tok.Scoped() {
		return nil
	}
	svc := serviceOf(toolName)
	if svc == "" || !tok.AllowsService(svc) {
		return fmt.Errorf("this token is not scoped for %s", toolName)
	}
	return nil
}
