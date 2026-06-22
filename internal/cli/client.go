// MCP HTTP client. All communication with the Mu instance happens via
// JSON-RPC over the /mcp endpoint.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a Mu MCP endpoint.
type Client struct {
	URL   string
	Token string
	HTTP  *http.Client
}

// NewClient builds a client from a resolved config.
func NewClient(cfg *ResolvedConfig) *Client {
	return &Client{
		URL:   strings.TrimRight(cfg.URL, "/"),
		Token: cfg.Token,
		HTTP:  &http.Client{Timeout: 120 * time.Second},
	}
}

// jsonrpcRequest is the JSON-RPC 2.0 request envelope.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcResponse is the JSON-RPC 2.0 response envelope.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool describes a single MCP tool as returned by tools/list.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema mirrors the JSON schema fragment the MCP server emits.
type InputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]SchemaField `json:"properties"`
	Required   []string               `json:"required"`
}

// SchemaField is a single property in an MCP tool's input schema.
type SchemaField struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ListTools fetches the full list of tools from the server.
func (c *Client) ListTools() ([]Tool, error) {
	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := c.call("tools/list", nil, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool invokes a tool and returns the content text. When the server
// marks the result as an error, the text is still returned alongside
// a non-nil error so the caller can surface whatever the server said.
func (c *Client) CallTool(name string, args map[string]any) (string, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := c.call("tools/call", params, &result); err != nil {
		return "", err
	}
	var text string
	for _, block := range result.Content {
		if block.Type == "text" {
			if text != "" {
				text += "\n"
			}
			text += block.Text
		}
	}
	if result.IsError {
		return text, fmt.Errorf("tool error: %s", text)
	}
	return text, nil
}

// Ping checks that the server is reachable and returns the protocol
// version string.
func (c *Client) Ping() error {
	var out json.RawMessage
	return c.call("ping", nil, &out)
}

// call is the low-level JSON-RPC sender.
func (c *Client) call(method string, params any, out any) error {
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		rawParams = b
	}
	reqBody := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  rawParams,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	endpoint := c.URL + "/mcp"
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusPaymentRequired {
		return fmt.Errorf("payment required (HTTP 402): insufficient credits. Top up at %s/wallet", c.URL)
	}
	if resp.StatusCode >= 400 && len(respBody) == 0 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, endpoint)
	}

	var rpcResp jsonrpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return fmt.Errorf("parse response: %w (body: %s)", err, trunc(string(respBody), 200))
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("%s", rpcResp.Error.Message)
	}
	if out != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, out); err != nil {
			return fmt.Errorf("parse result: %w", err)
		}
	}
	return nil
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
