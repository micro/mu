package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id"`
	Result  any         `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
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
	Type       string                `json:"type"`
	Properties map[string]mcpProperty `json:"properties,omitempty"`
	Required   []string              `json:"required,omitempty"`
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
	WalletOp    string // Wallet operation for credit gating (empty = free)
}

// QuotaCheck is called before executing a metered tool.
// It receives the HTTP request (for auth) and the wallet operation string.
// Returns (canProceed, creditCost, error).
// Set by main.go to wire in auth + wallet packages without import cycles.
var QuotaCheck func(r *http.Request, op string) (bool, int, error)

// ToolParam defines a parameter for an MCP tool
type ToolParam struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// tools is the list of MCP tools derived from API endpoints
var tools = []Tool{
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
		Path:        "/post",
		Params: []ToolParam{
			{Name: "id", Type: "string", Description: "The blog post ID", Required: true},
		},
	},
	{
		Name:        "blog_create",
		Description: "Create a new blog post",
		Method:      "POST",
		Path:        "/post",
		Params: []ToolParam{
			{Name: "title", Type: "string", Description: "Post title", Required: false},
			{Name: "content", Type: "string", Description: "Post content (minimum 50 characters)", Required: true},
		},
	},
	{
		Name:        "blog_update",
		Description: "Update an existing blog post (author only)",
		Method:      "PATCH",
		Path:        "/post",
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
		Path:        "/post",
		Params: []ToolParam{
			{Name: "id", Type: "string", Description: "The blog post ID to delete", Required: true},
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
}

// MCPHandler handles MCP protocol requests at /api/mcp
func MCPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(jsonrpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32600, Message: "Only POST method is supported"},
		})
		return
	}

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

	// Check wallet quota for metered tools
	if tool.WalletOp != "" && QuotaCheck != nil {
		canProceed, cost, err := QuotaCheck(originalReq, tool.WalletOp)
		if !canProceed {
			msg := fmt.Sprintf("Insufficient credits: %s requires %d credits", tool.Name, cost)
			if err != nil {
				msg = err.Error()
			}
			writeError(w, req.ID, -32000, msg)
			return
		}
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
