package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"mu/internal/auth"
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
	Name        string
	Description string
	Method      string
	Path        string
	Params      []ToolParam
	WalletOp    string                                          // Wallet operation for credit gating (empty = included)
	Handle      func(map[string]any) (string, error)            // Optional direct handler (bypasses HTTP dispatch)
	HandleAuth  func(map[string]any, string) (string, error)    // Like Handle but receives the account ID
}

// QuotaCheck is called before executing a metered tool.
// It receives the HTTP request (for auth) and the wallet operation string.
// Returns (canProceed, creditCost, error).
// Set by main.go to wire in auth + wallet packages without import cycles.
var QuotaCheck func(r *http.Request, op string) (bool, int, error)

// ToolGuard is called before executing any tool — used for tool-specific
// pre-checks (e.g. signup rate limiting per IP). Returning an error blocks
// the call and the error message is returned to the caller. Set by main.go.
var ToolGuard func(r *http.Request, toolName string) error

// PaymentRequiredResponse is called when quota check fails to build x402 payment
// requirements. Returns nil if x402 is not enabled. Set by main.go.
var PaymentRequiredResponse func(w http.ResponseWriter, op string, resource string)

// ToolParam defines a parameter for an MCP tool
type ToolParam struct {
	Name        string
	Type        string
	Description string
	Required    bool
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

// tools is the list of MCP tools derived from API endpoints
var tools = []Tool{
	{
		Name:        "me",
		Description: "Get the current authenticated user's identity, account ID, and admin status",
		Method:      "GET",
		Path:        "/session",
	},
	{
		Name:        "agent",
		Description: "Run the AI agent — plans which tools to use, executes them, and synthesises an answer. Use for complex queries that need multiple data sources.",
		Method:      "POST",
		Path:        "/agent/run",
		WalletOp:    "agent_query",
		Params: []ToolParam{
			{Name: "prompt", Type: "string", Description: "What you want the agent to do", Required: true},
		},
	},
	{
		Name:        "chat",
		Description: "Chat with AI assistant",
		Method:      "POST",
		Path:        "/chat",
		WalletOp:    "chat_query",
		Params: []ToolParam{
			{Name: "prompt", Type: "string", Description: "The message to send to the AI", Required: true},
		},
	},
	{
		Name:        "news",
		Description: "Read the latest news feed",
		Method:      "GET",
		Path:        "/news",
	},
	{
		Name:        "news_search",
		Description: "Search for news articles",
		Method:      "POST",
		Path:        "/news",
		WalletOp:    "news_search",
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "News search query", Required: true},
		},
	},
	{
		Name:        "blog_list",
		Description: "Get all blog posts",
		Method:      "GET",
		Path:        "/blog",
	},
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
		WalletOp:    "blog_create",
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
		Name:        "social",
		Description: "Get the latest social media posts from followed accounts",
		Method:      "GET",
		Path:        "/social",
	},
	{
		Name:        "social_search",
		Description: "Search social media posts",
		Method:      "POST",
		Path:        "/social",
		WalletOp:    "social_search",
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Search query for social posts", Required: true},
		},
	},
	{
		Name:        "video",
		Description: "Get the latest videos",
		Method:      "GET",
		Path:        "/video",
	},
	{
		Name:        "video_search",
		Description: "Search for videos",
		Method:      "POST",
		Path:        "/video",
		WalletOp:    "video_search",
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Video search query", Required: true},
		},
	},
	{
		Name:        "mail_read",
		Description: "Read mail inbox",
		Method:      "GET",
		Path:        "/mail",
	},
	{
		Name:        "mail_send",
		Description: "Send a mail message",
		Method:      "POST",
		Path:        "/mail",
		WalletOp:    "external_email",
		Params: []ToolParam{
			{Name: "to", Type: "string", Description: "Recipient username or email", Required: true},
			{Name: "subject", Type: "string", Description: "Message subject", Required: true},
			{Name: "body", Type: "string", Description: "Message body", Required: true},
		},
	},
	{
		Name:        "search",
		Description: "Search across all indexed content (posts, news, videos)",
		Method:      "GET",
		Path:        "/search",
		Params: []ToolParam{
			{Name: "q", Type: "string", Description: "Search query", Required: true},
		},
	},
	{
		Name:        "wallet_balance",
		Description: "Get wallet credit balance",
		Method:      "GET",
		Path:        "/wallet",
		Params: []ToolParam{
			{Name: "balance", Type: "string", Description: "Set to 1 to get balance", Required: false},
		},
	},
	{
		Name:        "wallet_transfer",
		Description: "Transfer credits to another user by username",
		Method:      "POST",
		Path:        "/wallet/transfer",
		Params: []ToolParam{
			{Name: "to", Type: "string", Description: "Recipient username", Required: true},
			{Name: "amount", Type: "number", Description: "Number of credits to transfer", Required: true},
		},
	},
	{
		Name:        "wallet_topup",
		Description: "Get available wallet topup payment methods with crypto deposit address and card payment tiers",
		Method:      "GET",
		Path:        "/wallet/topup",
	},
	{
		Name:        "work_list",
		Description: "List work posts — show (people sharing work) and tasks (bounties for work needed)",
		Method:      "GET",
		Path:        "/work",
		Params: []ToolParam{
			{Name: "kind", Type: "string", Description: "Filter by kind: task or show (default: all)", Required: false},
		},
	},
	{
		Name:        "work_post",
		Description: "Post work — show something you built or post a task with a credit cost",
		Method:      "POST",
		Path:        "/work/post",
		Params: []ToolParam{
			{Name: "kind", Type: "string", Description: "Post kind: show or task (default: show)", Required: false},
			{Name: "title", Type: "string", Description: "Title", Required: true},
			{Name: "description", Type: "string", Description: "Description of the work", Required: true},
			{Name: "link", Type: "string", Description: "URL or app slug (optional, for show posts)", Required: false},
			{Name: "cost", Type: "number", Description: "Budget in credits — max spend for agent (required for tasks)", Required: false},
		},
	},
	// Stream (console)
	{
		Name:        "stream",
		Description: "Read the platform event stream — user messages, agent responses, system events (markets, news, reminders)",
		Method:      "GET",
		Path:        "/stream",
	},
	{
		Name:        "stream_post",
		Description: "Post a message to the stream. Mention @micro to get an AI response. Costs 1 credit.",
		Method:      "POST",
		Path:        "/stream",
		WalletOp:    "social_post",
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
		WalletOp:    "places_search",
		Params: []ToolParam{
			{Name: "q", Type: "string", Description: "Search query (e.g. cafe, pharmacy, Boots)", Required: true},
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
		WalletOp:    "places_nearby",
		Params: []ToolParam{
			{Name: "address", Type: "string", Description: "Address or postcode to search near", Required: false},
			{Name: "lat", Type: "number", Description: "Latitude of the search location", Required: false},
			{Name: "lon", Type: "number", Description: "Longitude of the search location", Required: false},
			{Name: "radius", Type: "number", Description: "Search radius in metres, 100–5000 (default 500)", Required: false},
		},
	},
	{
		Name:        "weather_forecast",
		Description: "Get the weather forecast for a location. Returns current conditions, hourly and daily forecast. Optionally includes pollen data.",
		Method:      "GET",
		Path:        "/weather",
		WalletOp:    "weather_forecast",
		Params: []ToolParam{
			{Name: "lat", Type: "number", Description: "Latitude of the location", Required: true},
			{Name: "lon", Type: "number", Description: "Longitude of the location", Required: true},
			{Name: "pollen", Type: "string", Description: "Set to 1 to include pollen forecast (+1 credit)", Required: false},
		},
	},
	{
		Name:        "markets",
		Description: "Get live market prices for cryptocurrencies, futures, and commodities",
		Method:      "GET",
		Path:        "/markets",
		Params: []ToolParam{
			{Name: "category", Type: "string", Description: "Category of markets: crypto, futures, or commodities (default: crypto)", Required: false},
		},
	},
	{
		Name:        "reminder",
		Description: "Get today's daily Islamic reminder with verse, hadith, and name of Allah",
		Handle: func(args map[string]any) (string, error) {
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get("https://reminder.dev/api/daily")
			if err != nil {
				return "", fmt.Errorf("reminder API error: %v", err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return "", fmt.Errorf("reading reminder response: %v", err)
			}
			return string(body), nil
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
			url := "https://reminder.dev/api/quran/" + chapter
			if v, ok := args["verse"].(float64); ok && v > 0 {
				url += fmt.Sprintf("/%d", int(v))
			}
			resp, err := http.Get(url)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return string(b), nil
		},
	},
	{
		Name:        "hadith",
		Description: "Look up hadith from Sahih Al Bukhari. Pass a book number to get hadiths from that book.",
		Params: []ToolParam{
			{Name: "book", Type: "number", Description: "Book number", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			url := "https://reminder.dev/api/hadith"
			if b, ok := args["book"].(float64); ok && b > 0 {
				url += fmt.Sprintf("/%d", int(b))
			}
			resp, err := http.Get(url)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return string(b), nil
		},
	},
	{
		Name:        "quran_search",
		Description: "Search the Quran, Hadith, and names of Allah using semantic search. Ask a question in natural language.",
		Params: []ToolParam{
			{Name: "q", Type: "string", Description: "Question or search query", Required: true},
		},
		WalletOp: "search",
		Handle: func(args map[string]any) (string, error) {
			q, _ := args["q"].(string)
			if q == "" {
				return "", fmt.Errorf("query is required")
			}
			body := fmt.Sprintf(`{"q":%q}`, q)
			resp, err := http.Post("https://reminder.dev/api/search", "application/json", strings.NewReader(body))
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return string(b), nil
		},
	},
}

// mcpPostHandler handles MCP JSON-RPC POST requests
func mcpPostHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, nil, -32700, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	var req jsonrpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, nil, -32700, "Parse error")
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "initialize":
		handleInitialize(w, req)
	case "notifications/initialized":
		// Client acknowledgement, no response needed
		w.WriteHeader(http.StatusNoContent)
	case "tools/list":
		handleToolsList(w, req)
	case "tools/call":
		handleToolsCall(w, r, req)
	case "ping":
		writeResult(w, req.ID, map[string]any{})
	default:
		writeError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func handleInitialize(w http.ResponseWriter, req jsonrpcRequest) {
	result := mcpInitializeResult{
		ProtocolVersion: MCPVersion,
		ServerInfo: mcpServerInfo{
			Name:    "mu",
			Version: "1.0.0",
		},
		Capabilities: mcpCapabilities{
			Tools: &mcpToolCapability{},
		},
	}
	writeResult(w, req.ID, result)
}

func handleToolsList(w http.ResponseWriter, req jsonrpcRequest) {
	mcpTools := make([]mcpTool, 0, len(tools))
	for _, t := range tools {
		tool := mcpTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: mcpInputSchema{
				Type:       "object",
				Properties: make(map[string]mcpProperty),
			},
		}
		var required []string
		for _, p := range t.Params {
			tool.InputSchema.Properties[p.Name] = mcpProperty{
				Type:        p.Type,
				Description: p.Description,
			}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		if len(required) > 0 {
			tool.InputSchema.Required = required
		}
		mcpTools = append(mcpTools, tool)
	}
	writeResult(w, req.ID, mcpToolsListResult{Tools: mcpTools})
}

func handleToolsCall(w http.ResponseWriter, originalReq *http.Request, req jsonrpcRequest) {
	var params mcpToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -32602, "Invalid params")
		return
	}

	// Find the tool
	var tool *Tool
	for i := range tools {
		if tools[i].Name == params.Name {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		writeError(w, req.ID, -32602, fmt.Sprintf("Unknown tool: %s", params.Name))
		return
	}

	// Tool-specific pre-checks (e.g. signup rate limit per IP).
	if ToolGuard != nil {
		if err := ToolGuard(originalReq, tool.Name); err != nil {
			writeError(w, req.ID, -32000, err.Error())
			return
		}
	}

	// Check wallet quota for metered tools
	if tool.WalletOp != "" && QuotaCheck != nil {
		canProceed, cost, err := QuotaCheck(originalReq, tool.WalletOp)
		if !canProceed {
			// If x402 is enabled, return 402 with payment requirements
			if PaymentRequiredResponse != nil {
				PaymentRequiredResponse(w, tool.WalletOp, originalReq.URL.Path)
				return
			}
			msg := fmt.Sprintf("Insufficient credits: %s requires %d credits", tool.Name, cost)
			if err != nil {
				msg = err.Error()
			}
			writeError(w, req.ID, -32000, msg)
			return
		}
	}

	// Use custom handler if provided (e.g. auth tools)
	if tool.Handle != nil {
		text, err := tool.Handle(params.Arguments)
		result := mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: text}},
		}
		if err != nil {
			result.IsError = true
		}

		writeResult(w, req.ID, result)
		return
	}

	// Build the internal HTTP request
	path := tool.Path
	var bodyReader io.Reader

	if tool.Method == "GET" {
		// Add params as query string
		query := url.Values{}
		for k, v := range params.Arguments {
			query.Set(k, fmt.Sprintf("%v", v))
		}
		if len(query) > 0 {
			path += "?" + query.Encode()
		}
	} else {
		// POST/PATCH/DELETE: send params as JSON body
		bodyJSON, _ := json.Marshal(params.Arguments)
		bodyReader = strings.NewReader(string(bodyJSON))
	}

	internalReq, err := http.NewRequest(tool.Method, path, bodyReader)
	if err != nil {
		writeError(w, req.ID, -32603, "Failed to create request")
		return
	}

	// Set JSON headers
	internalReq.Header.Set("Accept", "application/json")
	internalReq.Header.Set("Content-Type", "application/json")

	// Forward authentication from the original request
	if c, err := originalReq.Cookie("session"); err == nil {
		internalReq.AddCookie(c)
	}
	if auth := originalReq.Header.Get("Authorization"); auth != "" {
		internalReq.Header.Set("Authorization", auth)
	}
	if token := originalReq.Header.Get(TokenHeader); token != "" {
		internalReq.Header.Set(TokenHeader, token)
	}

	// Execute via the default mux
	recorder := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(recorder, internalReq)

	result := mcpToolResult{
		Content: []mcpContent{{
			Type: "text",
			Text: recorder.Body.String(),
		}},
	}
	if recorder.Code >= 400 {
		result.IsError = true
	}
	writeResult(w, req.ID, result)
}

// ExecuteToolAs calls a tool on behalf of a user account (no HTTP request needed).
// Creates a temporary session for auth. Used by background agents.
func ExecuteToolAs(accountID, name string, args map[string]any) (string, bool, error) {
	sess, err := auth.CreateSession(accountID)
	if err != nil {
		return "", true, fmt.Errorf("failed to create session: %v", err)
	}

	req, _ := http.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return ExecuteTool(req, name, args)
}

// ExecuteTool calls a registered MCP tool with the given name and arguments,
// forwarding authentication from r. It does NOT check wallet quota — the caller
// is responsible for quota management.
// Returns the tool output text, whether the response is an error, and any Go error.
func ExecuteTool(r *http.Request, name string, args map[string]any) (string, bool, error) {
	var tool *Tool
	for i := range tools {
		if tools[i].Name == name {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		return "", true, fmt.Errorf("unknown tool: %s", name)
	}

	if tool.HandleAuth != nil {
		// Extract account ID from the request session
		accountID := ""
		if _, acc := auth.TrySession(r); acc != nil {
			accountID = acc.ID
		}
		text, err := tool.HandleAuth(args, accountID)
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
