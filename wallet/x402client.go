package wallet

// x402 payer client: call a tool on any MCP server and, if it answers 402,
// pay from the user's Base wallet and retry. Works against this instance and
// any other x402 MCP server added to the registry — the basis for an agent
// that spends its own wallet across servers.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"mu/internal/settings"
)

// Server is a named MCP endpoint the agent can call.
type Server struct {
	Name string `json:"name"`
	URL  string `json:"url"` // base URL; /mcp is appended
}

// Servers returns the configured MCP servers: always "self" (this instance),
// plus any in X402_SERVERS as "name=url,name2=url2".
func Servers() []Server {
	out := []Server{{Name: "self", URL: strings.TrimRight(settings.Get("APP_URL"), "/")}}
	for _, entry := range strings.Split(settings.Get("X402_SERVERS"), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, url, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		out = append(out, Server{Name: strings.TrimSpace(name), URL: strings.TrimRight(strings.TrimSpace(url), "/")})
	}
	return out
}

// ServerURL resolves a server name to its base URL (defaults to self).
func ServerURL(name string) string {
	for _, s := range Servers() {
		if strings.EqualFold(s.Name, name) {
			return s.URL
		}
	}
	if name == "" || strings.EqualFold(name, "self") {
		return strings.TrimRight(settings.Get("APP_URL"), "/")
	}
	return ""
}

var payClient = &http.Client{Timeout: 60 * time.Second}

// PayAndCallMCP calls tool on the MCP server at baseURL with args. If the server
// answers HTTP 402, it signs a payment from bw for a payable requirement and
// retries once. Returns the tool's text result. accountID is the authenticated
// caller, used only to meter spend against the daily cap — never to choose the
// source wallet (that is always bw).
func PayAndCallMCP(ctx context.Context, accountID, baseURL, tool string, args map[string]any, bw *BaseWallet) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/mcp"
	rpc := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	}

	status, body, err := postJSON(ctx, endpoint, rpc, "")
	if err != nil {
		return "", err
	}

	if status == http.StatusPaymentRequired {
		if bw == nil {
			return "", fmt.Errorf("payment required but no wallet")
		}
		req, perr := choosePayable(body)
		if perr != nil {
			return "", perr
		}
		// Bound the payment: a server's challenge can never authorise more than
		// the per-call/daily caps, so a misled agent can't drain the wallet.
		amt, ok := new(big.Int).SetString(strings.TrimSpace(req.MaxAmountRequired), 10)
		if !ok {
			return "", fmt.Errorf("invalid payment amount %q", req.MaxAmountRequired)
		}
		if err := checkAndRecordSpend(accountID, amt); err != nil {
			return "", err
		}
		payHeader, serr := SignX402Payment(bw, req)
		if serr != nil {
			return "", fmt.Errorf("sign payment: %w", serr)
		}
		status, body, err = postJSON(ctx, endpoint, rpc, payHeader)
		if err != nil {
			return "", err
		}
		if status == http.StatusPaymentRequired {
			return "", fmt.Errorf("payment rejected: %s", challengeError(body))
		}
	}

	return parseToolResult(body)
}

// choosePayable picks a requirement from a 402 body that this wallet can pay:
// a known EVM network with an EIP-712 domain in extra.
func choosePayable(body []byte) (PaymentRequirements, error) {
	var challenge struct {
		Accepts []PaymentRequirements `json:"accepts"`
	}
	if err := json.Unmarshal(body, &challenge); err != nil {
		return PaymentRequirements{}, fmt.Errorf("bad 402 body: %w", err)
	}
	for _, r := range challenge.Accepts {
		if _, ok := chainIDFor(r.Network); ok && r.Extra["name"] != "" && r.PayTo != "" {
			return r, nil
		}
	}
	return PaymentRequirements{}, fmt.Errorf("no payable requirement offered")
}

func challengeError(body []byte) string {
	var c struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &c)
	if c.Error != "" {
		return c.Error
	}
	return strings.TrimSpace(string(body))
}

// parseToolResult extracts text from a JSON-RPC tools/call response.
func parseToolResult(body []byte) (string, error) {
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("bad MCP response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("%s", resp.Error.Message)
	}
	var sb strings.Builder
	for _, c := range resp.Result.Content {
		sb.WriteString(c.Text)
	}
	return sb.String(), nil
}

func postJSON(ctx context.Context, url string, payload any, xPayment string) (int, []byte, error) {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if xPayment != "" {
		req.Header.Set("X-PAYMENT", xPayment)
	}
	resp, err := payClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, err
}
