package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/mcp", nil)
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
