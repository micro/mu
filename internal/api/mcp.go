package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mu/internal/service"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/usage"
	"mu/service/wallet"
)

// MCP protocol version
const MCPVersion = "2025-03-26"

var (
	reminderHTTPClient = &http.Client{Timeout: 10 * time.Second}
	reminderAPIBase    = "https://reminder.dev/api"
)

func reminderAPIURL(path string) string {
	return strings.TrimRight(reminderAPIBase, "/") + path
}

func getReminderAPI(path string) (string, error) {
	resp, err := reminderHTTPClient.Get(reminderAPIURL(path))
	if err != nil {
		return "", fmt.Errorf("reminder API error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("reminder API returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading reminder response: %w", err)
	}
	return string(body), nil
}

func postReminderAPI(path, contentType, body string) (string, error) {
	resp, err := reminderHTTPClient.Post(reminderAPIURL(path), contentType, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("reminder API error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("reminder API returned status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading reminder response: %w", err)
	}
	return string(b), nil
}

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
	Title       string      `json:"title,omitempty"` // display title for the visual card
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
	Card       func() string                                `json:"-"` // Optional visual card body, rendered from live data
}

// QuotaCheck is called before executing a metered tool.
// It receives the HTTP request (for auth) and the wallet operation string.
// Returns (canProceed, creditCost, error).
// Set by main.go to wire in auth + wallet packages without import cycles.
var QuotaCheck func(r *http.Request, op string) (bool, int, error)

// PaymentRequiredResponse is called when quota check fails to build x402 payment
// requirements. Returns nil if x402 is not enabled. Set by main.go.
var PaymentRequiredResponse func(w http.ResponseWriter, op string, resource string)

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
// operation for a tools/call to a metered tool, or "" if the call is free or is
// not a tools/call. Lets the HTTP layer gate x402 payments before dispatch,
// where auth and wallet packages are in scope.
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
func RegisterTool(t Tool) {
	tools = append(tools, t)
}

// RegisterToolWithAuth adds a tool that receives the authenticated account ID.
func RegisterToolWithAuth(t Tool, handler func(map[string]any, string) (string, error)) {
	t.HandleAuth = handler
	tools = append(tools, t)
}

// ToolDocs returns a formatted string documenting all registered tools
// and their parameters. Used by the app builder to give the AI accurate API info.
func ToolDocs() string {
	var sb strings.Builder
	sb.WriteString("Available platform APIs (accessed via mu.api.get/mu.api.post):\n\n")
	for _, t := range tools {
		if t.Path == "" && t.Handle == nil && t.HandleAuth == nil {
			continue
		}
		method := t.Method
		if method == "" {
			if t.Handle != nil || t.HandleAuth != nil {
				method = "TOOL"
			}
		}
		sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", t.Name, method, t.Description))
		if len(t.Params) > 0 {
			sb.WriteString("  Parameters:\n")
			for _, p := range t.Params {
				req := ""
				if p.Required {
					req = " (required)"
				}
				sb.WriteString(fmt.Sprintf("    - %s (%s): %s%s\n", p.Name, p.Type, p.Description, req))
			}
		}
	}
	return sb.String()
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
	{
		Name:        "chat",
		Description: "Chat with AI assistant",
		Method:      "POST",
		Path:        "/chat",
		WalletOp:    wallet.OpChatQuery,
		Params: []ToolParam{
			{Name: "prompt", Type: "string", Description: "The message to send to the AI", Required: true},
		},
	},
	{
		Name:        "news_search",
		Description: "Search for news articles",
		Method:      "POST",
		Path:        "/news",
		WalletOp:    wallet.OpNewsSearch,
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "News search query", Required: true},
		},
	},
	// blog_list, social, video, weather_forecast and markets are registered in
	// main.go as AI-first tools (clean Go handlers returning model-ready text),
	// not as page-backed entries here.
	{
		Name:        "blog_read",
		Description: "Read a specific blog post by ID",
		Method:      "GET",
		Path:        "/blog/post",
		Params: []ToolParam{
			{Name: "id", Type: "string", Description: "The blog post ID", Required: true},
		},
	},
	{
		Name:        "blog_create",
		Description: "Create a new blog post",
		Method:      "POST",
		Path:        "/blog/post",
		WalletOp:    wallet.OpBlogCreate,
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
		Description: "Delete a blog post (author only)",
		Method:      "DELETE",
		Path:        "/blog/post",
		Params: []ToolParam{
			{Name: "id", Type: "string", Description: "The blog post ID to delete", Required: true},
		},
	},
	{
		Name:        "social_search",
		Description: "Search social media posts",
		Method:      "POST",
		Path:        "/social",
		WalletOp:    wallet.OpSocialSearch,
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Search query for social posts", Required: true},
		},
	},
	{
		Name:        "video_search",
		Description: "Search for videos",
		Method:      "POST",
		Path:        "/video",
		WalletOp:    wallet.OpVideoSearch,
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Video search query", Required: true},
		},
	},
	{
		Name:        "mail_inbox",
		Aliases:     []string{"mail_read"},
		Description: "Read your mail inbox. Pass a tag to read only mail sent to that plus-address (you+tag@), which is how an agent reads its own mail rather than all of yours.",
		Method:      "GET",
		Path:        "/mail",
		Params: []ToolParam{
			{Name: "tag", Type: "string", Description: "Only mail sent to you+<tag>@ — omit for the whole inbox", Required: false},
		},
	},
	{
		Name:        "mail_send",
		AccountOnly: true, // a funded wallet is not accountable for the domain
		Description: "Send a mail message",
		Method:      "POST",
		Path:        "/mail",
		WalletOp:    wallet.OpExternalEmail,
		Params: []ToolParam{
			{Name: "to", Type: "string", Description: "Recipient username or email", Required: true},
			{Name: "subject", Type: "string", Description: "Message subject", Required: true},
			{Name: "body", Type: "string", Description: "Message body", Required: true},
		},
	},
	// wallet_balance is registered in main.go, where the wallet package is
	// reachable — it answers credits, deposit address and USDC in one call.
	//
	// Three tools used to live here. wallet_transfer moved credits to another
	// user by username, irreversibly, in a single call with no confirmation.
	// The same agent holds mail_inbox, news_read, web_fetch and db_list — four
	// ways to read text somebody else wrote — and nothing downstream could tell
	// "the user asked" from "the agent read it in an email". Transferring
	// credits is a thing a person does a handful of times, deliberately, and
	// /wallet/transfer already does it with a form and a CSRF token. It is not
	// a capability an agent should be granted, so it is not a tool.
	//
	// wallet_topup returned card tiers to a caller that cannot complete a card
	// purchase. Its only real output was "tell your human where to go", which
	// belongs in the message you get when a call fails for want of credits, not
	// in a tool an agent has to know to call.
	// Stream (console)
	{
		Name:        "stream_list",
		Aliases:     []string{"stream"},
		Description: "Read the platform event stream — user messages, agent responses, system events (markets, news, reminders)",
		Method:      "GET",
		Path:        "/stream",
	},
	{
		Name:        "stream_post",
		Description: "Post a message to the stream. Mention @micro to get an AI response.",
		Method:      "POST",
		Path:        "/stream",
		// OpStreamPost, matching what the stream service declares. This said
		// OpSocialPost, so CREDIT_COST_STREAM_POST did not price the tool named
		// stream_post — an operator setting it would have seen no effect. The
		// description also claimed "Costs 1 credit" while both operations
		// default to 0, so the page rendered "Included" beside a line saying
		// otherwise. Prices are rendered from the operation; no description
		// states one.
		WalletOp: wallet.OpStreamPost,
		Params: []ToolParam{
			{Name: "content", Type: "string", Description: "Message text (max 1024 chars). Use @micro to invoke the AI agent.", Required: true},
		},
	},
	// Content controls
	{
		Name:        "flag",
		Description: "Flag content for moderation",
		Method:      "POST",
		Path:        "/app/flag",
		Params: []ToolParam{
			{Name: "type", Type: "string", Description: "Content type (e.g. post, work, app)", Required: true},
			{Name: "id", Type: "string", Description: "Content ID", Required: true},
		},
	},
	{
		Name:        "save",
		Description: "Bookmark content for later",
		Method:      "POST",
		Path:        "/app/save",
		Params: []ToolParam{
			{Name: "type", Type: "string", Description: "Content type (e.g. post, work, app)", Required: true},
			{Name: "id", Type: "string", Description: "Content ID", Required: true},
		},
	},
	{
		Name:        "unsave",
		Description: "Remove a saved bookmark",
		Method:      "POST",
		Path:        "/app/unsave",
		Params: []ToolParam{
			{Name: "type", Type: "string", Description: "Content type", Required: true},
			{Name: "id", Type: "string", Description: "Content ID", Required: true},
		},
	},
	{
		Name:        "dismiss",
		Description: "Hide content from your view",
		Method:      "POST",
		Path:        "/app/dismiss",
		Params: []ToolParam{
			{Name: "type", Type: "string", Description: "Content type", Required: true},
			{Name: "id", Type: "string", Description: "Content ID", Required: true},
		},
	},
	{
		Name:        "block_user",
		Description: "Block a user — hides all their content from your view",
		Method:      "POST",
		Path:        "/app/block",
		Params: []ToolParam{
			{Name: "user", Type: "string", Description: "User ID to block", Required: true},
		},
	},
	{
		Name:        "unblock_user",
		Description: "Unblock a previously blocked user",
		Method:      "POST",
		Path:        "/app/unblock",
		Params: []ToolParam{
			{Name: "user", Type: "string", Description: "User ID to unblock", Required: true},
		},
	},
	{
		Name:        "places_search",
		Description: "Search for places by name or category, optionally near a location",
		Method:      "POST",
		Path:        "/places/search",
		WalletOp:    wallet.OpPlacesSearch,
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Search query (e.g. cafe, pharmacy, Boots)", Required: true},
			{Name: "near", Type: "string", Description: "Location name or address to search near", Required: false},
			{Name: "near_lat", Type: "number", Description: "Latitude of the search location", Required: false},
			{Name: "near_lon", Type: "number", Description: "Longitude of the search location", Required: false},
			{Name: "radius", Type: "number", Description: "Search radius in metres, 100–5000 (default 1000)", Required: false},
		},
	},
	{
		Name:        "places_nearby",
		Description: "Find all places of interest near a given location",
		Method:      "POST",
		Path:        "/places/nearby",
		WalletOp:    wallet.OpPlacesNearby,
		Params: []ToolParam{
			{Name: "address", Type: "string", Description: "Address or postcode to search near", Required: false},
			{Name: "lat", Type: "number", Description: "Latitude of the search location", Required: false},
			{Name: "lon", Type: "number", Description: "Longitude of the search location", Required: false},
			{Name: "radius", Type: "number", Description: "Search radius in metres, 100–5000 (default 500)", Required: false},
		},
	},
	{
		Name:        "islam_today",
		Aliases:     []string{"islam", "reminder"},
		Description: "Get today's daily Islamic reminder with verse, hadith, and name of Allah",
		Handle: func(args map[string]any) (string, error) {
			return getReminderAPI("/daily")
		},
	},
	{
		Name:        "quran",
		Description: "Look up a Quran chapter or verse. Pass chapter number (1-114) and optionally a verse number.",
		Params: []ToolParam{
			{Name: "chapter", Type: "number", Description: "Chapter number (1-114)", Required: true},
			{Name: "verse", Type: "number", Description: "Verse number (optional, returns full chapter if omitted)", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			chapter := ""
			if c, ok := args["chapter"].(float64); ok {
				chapter = fmt.Sprintf("%d", int(c))
			}
			if chapter == "" {
				return "", fmt.Errorf("chapter is required")
			}
			path := "/quran/" + chapter
			if v, ok := args["verse"].(float64); ok && v > 0 {
				path += fmt.Sprintf("/%d", int(v))
			}
			return getReminderAPI(path)
		},
	},
	{
		Name:        "hadith",
		Description: "Look up hadith from Sahih Al Bukhari. Pass a book number to get hadiths from that book.",
		Params: []ToolParam{
			{Name: "book", Type: "number", Description: "Book number", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			path := "/hadith"
			if b, ok := args["book"].(float64); ok && b > 0 {
				path += fmt.Sprintf("/%d", int(b))
			}
			return getReminderAPI(path)
		},
	},
	{
		Name:        "quran_search",
		Description: "Search the Quran, Hadith, and names of Allah using semantic search. Ask a question in natural language.",
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Question or search query", Required: true},
		},
		WalletOp: wallet.OpQuranSearch,
		Handle: func(args map[string]any) (string, error) {
			q := QueryArg(args)
			if q == "" {
				return "", fmt.Errorf("query is required")
			}
			body := fmt.Sprintf(`{"q":%q}`, q)
			return postReminderAPI("/search", "application/json", body)
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

// WalletPayer is set by main.go to wallet.PayerFrom, which reads the settled
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
			return "Authentication required", true, err
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

func writeResult(w http.ResponseWriter, id any, result any) {
	json.NewEncoder(w).Encode(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeError(w http.ResponseWriter, id any, code int, message string) {
	json.NewEncoder(w).Encode(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	})
}
