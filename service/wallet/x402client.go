package wallet

// x402 payer client: call a tool on any MCP server and, if it answers 402,
// pay from the user's Base wallet and retry. Works against this instance and
// any other x402 MCP server added to the registry — the basis for an agent
// that spends its own wallet across servers.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/internal/origin"
	"mu/internal/settings"

	"mu/internal/x402"
)

// Server402 is a named MCP endpoint this wallet may pay.
type Server402 struct {
	Name string `json:"name" description:"The short name to pass to wallet_pay"`
	URL  string `json:"url" description:"Its base URL; /mcp is appended"`
}

// PaymentCallResult is a tool result plus the payment diagnostics exposed by
// the server after a paid retry. Settlement is nil for free calls.
type PaymentCallResult struct {
	Text               string
	Settlement         *x402.SettleResponse
	ExtensionResponses string
}

// Servers returns the configured MCP servers: always "self" (this instance),
// plus any in X402_SERVERS as "name=url,name2=url2".
//
// A closed list rather than "any URL the caller names", and that is the point
// rather than an omission: an agent that can pay a URL it read in a document
// has a wallet belonging to whoever writes the documents.
//
// It was unreachable for a long time — kept whole on the argument that half a
// payer is worse than none — and wallet_list and wallet_pay are what it was
// being kept for.
func Servers() []Server402 {
	out := []Server402{{Name: "self", URL: selfURL()}}
	for _, entry := range strings.Split(settings.Get("X402_SERVERS"), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, url, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		out = append(out, Server402{Name: strings.TrimSpace(name), URL: strings.TrimRight(strings.TrimSpace(url), "/")})
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
		return selfURL()
	}
	return ""
}

// selfURL is this instance's own address.
func selfURL() string {
	if v := strings.TrimSpace(settings.Get("APP_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return origin.Self()
}

var payClient = &http.Client{Timeout: 60 * time.Second}

// PayAndCallMCP preserves the long-standing text-only API for agents and other
// callers that do not care about payment diagnostics.
func PayAndCallMCP(ctx context.Context, accountID, baseURL, tool string, args map[string]any, bw *BaseWallet) (string, error) {
	res, err := PayAndCallMCPWithReceipt(ctx, accountID, baseURL, tool, args, bw)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// PayAndCallMCPWithReceipt is PayAndCallMCP plus the settlement receipt and any
// x402 extension response headers returned after payment. This is primarily for
// diagnostics: Bazaar indexing acknowledgements are carried in
// EXTENSION-RESPONSES, while PAYMENT-RESPONSE proves the payment itself settled.
func PayAndCallMCPWithReceipt(ctx context.Context, accountID, baseURL, tool string, args map[string]any, bw *BaseWallet) (PaymentCallResult, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/mcp"
	rpc := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	}

	status, body, headers, err := postJSON(ctx, endpoint, rpc, "", bw)
	if err != nil {
		return PaymentCallResult{}, err
	}

	paid := false
	if status == http.StatusPaymentRequired {
		if bw == nil {
			return PaymentCallResult{}, fmt.Errorf("payment required but no wallet")
		}
		req, perr := choosePayable(body)
		if perr != nil {
			return PaymentCallResult{}, perr
		}
		amt, ok := new(big.Int).SetString(req.AmountAtomic(), 10)
		if !ok {
			return PaymentCallResult{}, fmt.Errorf("invalid payment amount %q", req.AmountAtomic())
		}
		if err := checkAndRecordSpend(accountID, amt); err != nil {
			return PaymentCallResult{}, err
		}
		payHeader, serr := SignX402Payment(bw, req)
		if serr != nil {
			return PaymentCallResult{}, fmt.Errorf("sign payment: %w", serr)
		}
		status, body, headers, err = postJSON(ctx, endpoint, rpc, payHeader, bw)
		if err != nil {
			return PaymentCallResult{}, err
		}
		if status == http.StatusPaymentRequired {
			return PaymentCallResult{}, fmt.Errorf("payment rejected: %s", challengeError(body))
		}
		paid = true
	}

	text, err := parseToolResult(body)
	if err != nil {
		return PaymentCallResult{}, err
	}
	out := PaymentCallResult{Text: text}
	if paid {
		out.Settlement = settlementFromHeaders(headers)
		out.ExtensionResponses = strings.TrimSpace(headers.Get("EXTENSION-RESPONSES"))
	}
	return out, nil
}

func settlementFromHeaders(h http.Header) *x402.SettleResponse {
	value := firstHeader(h, x402.HeaderPaymentRespV2, x402.HeaderPaymentRespV1)
	if value == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil
	}
	var settle x402.SettleResponse
	if err := json.Unmarshal(raw, &settle); err != nil {
		return nil
	}
	return &settle
}

func firstHeader(h http.Header, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(h.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

// choosePayable picks a requirement from a 402 body that this wallet can pay.
func choosePayable(body []byte) (x402.PaymentRequirements, error) {
	var challenge struct {
		Accepts []x402.PaymentRequirements `json:"accepts"`
	}
	if err := json.Unmarshal(body, &challenge); err != nil {
		return x402.PaymentRequirements{}, fmt.Errorf("bad 402 body: %w", err)
	}
	for _, r := range challenge.Accepts {
		if _, ok := chainIDFor(r.Network); ok && r.Extra["name"] != "" && r.PayTo != "" {
			return r, nil
		}
	}
	return x402.PaymentRequirements{}, fmt.Errorf("no payable requirement offered")
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

func postJSON(ctx context.Context, endpoint string, payload any, xPayment string, bw *BaseWallet) (int, []byte, http.Header, error) {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if bw != nil {
		if u, uerr := url.Parse(endpoint); uerr == nil {
			if h, herr := SignAuth(bw, u.Host); herr == nil {
				req.Header.Set("Authorization", h)
			}
	}
	}
	if xPayment != "" {
		req.Header.Set("X-PAYMENT", xPayment)
	}
	resp, err := payClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, resp.Header.Clone(), err
}
