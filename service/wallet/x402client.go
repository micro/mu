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
// the server after a paid retry. Settlement is nil for free calls or a failed
// settlement receipt; PaymentError explains the latter when available.
type PaymentCallResult struct {
	Text               string
	Settlement         *x402.SettleResponse
	PaymentError       string
	ExtensionResponses string
}

// Servers returns the configured MCP servers: always "self" (this instance),
// plus any in X402_SERVERS as "name=url,name2=url2".
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

func selfURL() string {
	if v := strings.TrimSpace(settings.Get("APP_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return origin.Self()
}

var payClient = &http.Client{Timeout: 60 * time.Second}

func PayAndCallMCP(ctx context.Context, accountID, baseURL, tool string, args map[string]any, bw *BaseWallet) (string, error) {
	res, err := PayAndCallMCPWithReceipt(ctx, accountID, baseURL, tool, args, bw)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

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
		req, resource, extensions, perr := choosePayableChallenge(body)
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
		payHeader, serr := SignX402PaymentWithContext(bw, req, resource, extensions)
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
		out.Settlement, out.PaymentError = settlementFromHeaders(headers)
		out.ExtensionResponses = strings.TrimSpace(headers.Get("EXTENSION-RESPONSES"))
	}
	return out, nil
}

func settlementFromHeaders(h http.Header) (*x402.SettleResponse, string) {
	value := firstHeader(h, x402.HeaderPaymentRespV2, x402.HeaderPaymentRespV1)
	if value == "" {
		return nil, "no PAYMENT-RESPONSE returned by server"
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, "invalid PAYMENT-RESPONSE encoding"
	}
	var settle x402.SettleResponse
	if err := json.Unmarshal(raw, &settle); err != nil {
		return nil, "invalid PAYMENT-RESPONSE JSON"
	}
	if !settle.Success {
		return nil, firstNonEmpty(settle.Message, settle.ErrorReason, "settlement reported failure")
	}
	if strings.TrimSpace(settle.Transaction) == "" || strings.TrimSpace(settle.Network) == "" {
		return nil, "incomplete PAYMENT-RESPONSE: missing transaction or network"
	}
	return &settle, ""
}

func firstHeader(h http.Header, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(h.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// choosePayableChallenge selects a payable requirement while preserving the
// v2 resource and extension declarations. The x402 v2 client contract requires
// these declarations to be echoed in PaymentPayload; dropping them makes
// extension processors such as Bazaar see an empty payload even though the
// server advertised metadata in its 402 response.
func choosePayableChallenge(body []byte) (x402.PaymentRequirements, map[string]any, map[string]any, error) {
	var challenge struct {
		Accepts    []x402.PaymentRequirements `json:"accepts"`
		Resource   map[string]any              `json:"resource"`
		Extensions map[string]any              `json:"extensions"`
	}
	if err := json.Unmarshal(body, &challenge); err != nil {
		return x402.PaymentRequirements{}, nil, nil, fmt.Errorf("bad 402 body: %w", err)
	}
	for _, r := range challenge.Accepts {
		if _, ok := chainIDFor(r.Network); ok && r.Extra["name"] != "" && r.PayTo != "" {
			return r, challenge.Resource, challenge.Extensions, nil
		}
	}
	return x402.PaymentRequirements{}, nil, nil, fmt.Errorf("no payable requirement offered")
}

func choosePayable(body []byte) (x402.PaymentRequirements, error) {
	req, _, _, err := choosePayableChallenge(body)
	return req, err
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
