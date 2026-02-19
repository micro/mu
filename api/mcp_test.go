package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPHandler_GETReturnsPage(t *testing.T) {
	req := httptest.NewRequest("GET", "/mcp", nil)
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "MCP") {
		t.Error("Expected MCP page content in GET response")
	}
	if !strings.Contains(body, "tools/list") {
		t.Error("Expected tools/list example in MCP page")
	}
}

func TestMCPHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("DELETE", "/mcp", nil)
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("Expected error response")
	}
}

func TestMCPHandler_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("Expected error response")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("Expected parse error code -32700, got %d", resp.Error.Code)
	}
}

func TestMCPHandler_Initialize(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"test","version":"1.0"},"capabilities":{}}}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("Unexpected error: %s", resp.Error.Message)
	}
	if resp.ID != float64(1) {
		t.Errorf("Expected ID 1, got %v", resp.ID)
	}

	// Check result fields
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("Expected result to be an object")
	}
	if result["protocolVersion"] != MCPVersion {
		t.Errorf("Expected protocol version %s, got %v", MCPVersion, result["protocolVersion"])
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("Expected serverInfo to be an object")
	}
	if serverInfo["name"] != "mu" {
		t.Errorf("Expected server name 'mu', got %v", serverInfo["name"])
	}
}

func TestMCPHandler_ToolsList(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("Unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("Expected result to be an object")
	}
	toolsList, ok := result["tools"].([]any)
	if !ok {
		t.Fatal("Expected tools to be an array")
	}
	if len(toolsList) == 0 {
		t.Error("Expected at least one tool")
	}

	// Verify expected tools exist
	expectedTools := map[string]bool{
		"chat": false, "news": false, "news_search": false,
		"blog_list": false, "blog_read": false, "blog_create": false,
		"video": false, "video_search": false, "search": false,
	}
	for _, item := range toolsList {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tool["name"].(string)
		if _, exists := expectedTools[name]; exists {
			expectedTools[name] = true
		}

		// Verify tool has required fields
		if _, ok := tool["description"]; !ok {
			t.Errorf("Tool %s missing description", name)
		}
		if _, ok := tool["inputSchema"]; !ok {
			t.Errorf("Tool %s missing inputSchema", name)
		}
	}
	for name, found := range expectedTools {
		if !found {
			t.Errorf("Expected tool %q not found in tools list", name)
		}
	}
}

func TestMCPHandler_ToolsCallUnknown(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"unknown_tool","arguments":{}}}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("Expected error for unknown tool")
	}
	if !strings.Contains(resp.Error.Message, "unknown_tool") {
		t.Errorf("Expected error to mention tool name, got: %s", resp.Error.Message)
	}
}

func TestMCPHandler_Ping(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":4,"method":"ping"}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("Unexpected error: %s", resp.Error.Message)
	}
	if resp.ID != float64(4) {
		t.Errorf("Expected ID 4, got %v", resp.ID)
	}
}

func TestMCPHandler_MethodNotFound(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":5,"method":"unknown/method"}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("Expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("Expected method not found code -32601, got %d", resp.Error.Code)
	}
}

func TestMCPHandler_ToolsCallInvalidParams(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":"invalid"}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("Expected error for invalid params")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("Expected invalid params code -32602, got %d", resp.Error.Code)
	}
}

func TestMCPHandler_ToolsCallForwardsAuth(t *testing.T) {
	// Register a test handler that checks auth headers
	var receivedAuth string
	var receivedToken string
	http.HandleFunc("/test-mcp-auth", func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedToken = r.Header.Get(TokenHeader)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	// Add a temporary tool for this test
	tools = append(tools, Tool{
		Name:        "test_auth",
		Description: "Test auth forwarding",
		Method:      "GET",
		Path:        "/test-mcp-auth",
	})
	defer func() { tools = tools[:len(tools)-1] }()

	body := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"test_auth","arguments":{}}}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token-123")
	req.Header.Set(TokenHeader, "micro-token-456")
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	if receivedAuth != "Bearer test-token-123" {
		t.Errorf("Expected Authorization header forwarded, got %q", receivedAuth)
	}
	if receivedToken != "micro-token-456" {
		t.Errorf("Expected X-Micro-Token header forwarded, got %q", receivedToken)
	}
}

func TestMCPHandler_NotificationsInitialized(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}
}

func TestToolInputSchemaRequired(t *testing.T) {
	// Verify that tools with required params have them listed
	for _, tool := range tools {
		schema := mcpInputSchema{
			Type:       "object",
			Properties: make(map[string]mcpProperty),
		}
		var required []string
		for _, p := range tool.Params {
			schema.Properties[p.Name] = mcpProperty{
				Type:        p.Type,
				Description: p.Description,
			}
			if p.Required {
				required = append(required, p.Name)
			}
		}

		// Tools like "chat" should have required params
		if tool.Name == "chat" {
			if len(required) == 0 {
				t.Errorf("Tool %q should have required params", tool.Name)
			}
			found := false
			for _, r := range required {
				if r == "prompt" {
					found = true
				}
			}
			if !found {
				t.Error("Chat tool should require 'prompt' param")
			}
		}
	}
}

func TestMCPHandler_QuotaCheckBlocks(t *testing.T) {
	// Set up QuotaCheck to reject
	origQuotaCheck := QuotaCheck
	QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		return false, 3, nil
	}
	defer func() { QuotaCheck = origQuotaCheck }()

	body := `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"chat","arguments":{"prompt":"hello"}}}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("Expected error when quota check fails")
	}
	if resp.Error.Code != -32000 {
		t.Errorf("Expected quota error code -32000, got %d", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "credits") {
		t.Errorf("Expected credits-related error message, got: %s", resp.Error.Message)
	}
}

func TestMCPHandler_QuotaCheckAllows(t *testing.T) {
	// Set up QuotaCheck to allow
	origQuotaCheck := QuotaCheck
	QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		return true, 3, nil
	}
	defer func() { QuotaCheck = origQuotaCheck }()

	// Register a test handler
	http.HandleFunc("/test-quota-pass", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	origTools := make([]Tool, len(tools))
	copy(origTools, tools)
	tools = append(tools, Tool{
		Name:     "test_quota_pass",
		Method:   "GET",
		Path:     "/test-quota-pass",
		WalletOp: "chat_query",
	})
	defer func() { tools = origTools }()

	body := `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"test_quota_pass","arguments":{}}}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("Unexpected error: %s", resp.Error.Message)
	}
}

func TestMCPHandler_FreeToolsSkipQuotaCheck(t *testing.T) {
	// Set up QuotaCheck that should NOT be called for free tools
	origQuotaCheck := QuotaCheck
	quotaCalled := false
	QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		quotaCalled = true
		return false, 0, nil // Would block if called
	}
	defer func() { QuotaCheck = origQuotaCheck }()

	// Register a free test handler
	http.HandleFunc("/test-free-tool", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"free":true}`))
	})
	origTools := make([]Tool, len(tools))
	copy(origTools, tools)
	tools = append(tools, Tool{
		Name:   "test_free",
		Method: "GET",
		Path:   "/test-free-tool",
		// No WalletOp = free
	})
	defer func() { tools = origTools }()

	body := `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"test_free","arguments":{}}}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	if quotaCalled {
		t.Error("QuotaCheck should not be called for free tools (no WalletOp)")
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("Unexpected error: %s", resp.Error.Message)
	}
}

func TestMCPHandler_CustomHandler(t *testing.T) {
	// Register a tool with a custom handler
	origTools := make([]Tool, len(tools))
	copy(origTools, tools)
	RegisterTool(Tool{
		Name:        "test_custom",
		Description: "Custom handler test",
		Params: []ToolParam{
			{Name: "msg", Type: "string", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			msg, _ := args["msg"].(string)
			return `{"echo":"` + msg + `"}`, nil
		},
	})
	defer func() { tools = origTools }()

	body := `{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"test_custom","arguments":{"msg":"hello"}}}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("Unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("Expected result to be an object")
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("Expected content array")
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatal("Expected content item to be an object")
	}
	text, _ := first["text"].(string)
	if !strings.Contains(text, "hello") {
		t.Errorf("Expected echoed message, got: %s", text)
	}
}

func TestMCPHandler_CustomHandlerError(t *testing.T) {
	origTools := make([]Tool, len(tools))
	copy(origTools, tools)
	RegisterTool(Tool{
		Name:        "test_custom_err",
		Description: "Custom handler error test",
		Handle: func(args map[string]any) (string, error) {
			return `{"error":"something went wrong"}`, fmt.Errorf("something went wrong")
		},
	})
	defer func() { tools = origTools }()

	body := `{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"test_custom_err","arguments":{}}}`
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(body))
	w := httptest.NewRecorder()

	MCPHandler(w, req)

	var resp jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// Custom handler errors return as tool result with isError=true, not JSON-RPC error
	if resp.Error != nil {
		t.Fatalf("Expected tool result, not JSON-RPC error")
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("Expected result object")
	}
	isError, _ := result["isError"].(bool)
	if !isError {
		t.Error("Expected isError=true for custom handler error")
	}
}

func TestMCPHandler_MeteredToolsHaveWalletOp(t *testing.T) {
	// Verify that metered tools have WalletOp set
	expected := map[string]string{
		"chat":         "chat_query",
		"news_search":  "news_search",
		"video_search": "video_search",
		"mail_send":    "external_email",
	}
	for _, tool := range tools {
		if expectedOp, ok := expected[tool.Name]; ok {
			if tool.WalletOp != expectedOp {
				t.Errorf("Tool %q: expected WalletOp %q, got %q", tool.Name, expectedOp, tool.WalletOp)
			}
		}
	}

	// Verify free tools don't have WalletOp
	freeTtools := []string{"news", "blog_list", "blog_read", "video", "search"}
	for _, tool := range tools {
		for _, free := range freeTtools {
			if tool.Name == free && tool.WalletOp != "" {
				t.Errorf("Free tool %q should not have WalletOp, got %q", tool.Name, tool.WalletOp)
			}
		}
	}
}

func TestRegisterTool(t *testing.T) {
	origLen := len(tools)
	origTools := make([]Tool, len(tools))
	copy(origTools, tools)

	RegisterTool(Tool{
		Name:        "test_register",
		Description: "Test tool registration",
	})
	defer func() { tools = origTools }()

	if len(tools) != origLen+1 {
		t.Errorf("Expected %d tools after registration, got %d", origLen+1, len(tools))
	}

	found := false
	for _, tool := range tools {
		if tool.Name == "test_register" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Registered tool not found in tools list")
	}
}
