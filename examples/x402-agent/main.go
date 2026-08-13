// An agent that pays for a tool call, using nothing of Mu's.
//
// The question this answers is the one a developer actually asks: I have a
// wallet with some USDC in it, there is a server that charges per call — what
// do I write? Every previous answer we had was `mu x402 call`, which is our
// binary paying our server, and proves only that we can talk to ourselves.
//
// So this deliberately imports no Mu package. It is a separate Go module, and
// its only dependency is the x402 Foundation's own SDK. Everything it knows
// about this instance it learns from the 402 response, which means the same
// file pays any conformant x402 server — change the -server flag and it works
// against somebody else's. That portability is the point. An example that
// needed our client library would be advertising a lock-in we do not have.
//
//	export X402_PRIVATE_KEY=0x...              # a funded Base wallet
//	go run . -tool web_search -arg query=x402
//
// What happens on the wire, in four steps:
//
//  1. POST the MCP tools/call to /mcp with no credentials.
//  2. The server answers 402 with an `accepts` list — price, asset, payTo.
//  3. The SDK signs an EIP-3009 transfer authorisation for that exact amount.
//  4. It retries with the payment header, and the tool answers.
//
// Step 3 is a signature, not a transfer: nothing moves until the server hands
// it to a facilitator to settle. So the wallet needs USDC on Base but never
// needs ETH for gas, which is what makes this usable by something with no
// human attached.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	x402 "github.com/x402-foundation/x402/go"
	x402http "github.com/x402-foundation/x402/go/http"
	evmexact "github.com/x402-foundation/x402/go/mechanisms/evm/exact/client"
	evmexactv1 "github.com/x402-foundation/x402/go/mechanisms/evm/exact/v1/client"
	evmsigners "github.com/x402-foundation/x402/go/signers/evm"
)

// argList collects repeated -arg key=value flags into the tool's arguments.
type argList map[string]any

func (a argList) String() string { return fmt.Sprint(map[string]any(a)) }

func (a argList) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("expected key=value, got %q", v)
	}
	a[k] = val
	return nil
}

func main() {
	var (
		server = flag.String("server", "https://micro.mu", "base URL of the x402 MCP server")
		tool   = flag.String("tool", "web_search", "tool to call")
		args   = argList{}
	)
	flag.Var(args, "arg", "tool argument as key=value (repeatable)")
	flag.Parse()

	// The key is read from the environment and never from a flag, so it does
	// not end up in shell history or in the process list where any other user
	// on the box can read it.
	key := strings.TrimSpace(os.Getenv("X402_PRIVATE_KEY"))
	if key == "" {
		fmt.Fprintln(os.Stderr, "set X402_PRIVATE_KEY to a Base wallet holding USDC")
		os.Exit(2)
	}

	signer, err := evmsigners.NewClientSignerFromPrivateKey(key)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad key:", err)
		os.Exit(1)
	}
	fmt.Println("paying from", signer.Address())

	// Two registrations because there are two versions of the challenge in the
	// wild, and which one you meet is the server operator's choice rather than
	// yours. x402 v1 names its networks "base"; v2 uses the CAIP-2 id
	// "eip155:8453" and is matched here by wildcard. Registering both costs one
	// line and means this does not break against a server that upgrades.
	client := x402.Newx402Client().
		RegisterV1("base", evmexactv1.NewExactEvmSchemeV1(signer)).
		Register("eip155:*", evmexact.NewExactEvmScheme(signer, nil))

	// From here it is an ordinary *http.Client. The 402, the signature and the
	// retry all happen inside RoundTrip, which is why the call below has no
	// payment code in it at all.
	httpClient := x402http.WrapHTTPClientWithPayment(
		&http.Client{Timeout: 60 * time.Second},
		x402http.Newx402HTTPClient(client),
	)

	rpc, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": *tool, "arguments": map[string]any(args)},
	})

	endpoint := strings.TrimRight(*server, "/") + "/mcp"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(rpc))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "call failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// A settlement header means it was actually paid for. Printing the
	// transaction is the difference between believing the payment happened and
	// being able to go and look at it.
	if s := settlement(resp); s != "" {
		fmt.Println("settled:", s)
	}

	text, err := toolText(body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, strings.TrimSpace(string(body)))
		os.Exit(1)
	}
	fmt.Println(text)
}

// settlement decodes the server's payment receipt, which it returns base64 in
// X-PAYMENT-RESPONSE (v1) or PAYMENT-RESPONSE (v2).
func settlement(resp *http.Response) string {
	raw := resp.Header.Get("X-PAYMENT-RESPONSE")
	if raw == "" {
		raw = resp.Header.Get("PAYMENT-RESPONSE")
	}
	if raw == "" {
		return ""
	}
	dec, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return ""
	}
	var s struct {
		Transaction string `json:"transaction"`
		Network     string `json:"network"`
		Payer       string `json:"payer"`
	}
	if err := json.Unmarshal(dec, &s); err != nil || s.Transaction == "" {
		return ""
	}
	return fmt.Sprintf("%s on %s", s.Transaction, s.Network)
}

// toolText pulls the text out of a JSON-RPC tools/call response.
func toolText(body []byte) (string, error) {
	var r struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("unexpected response: %w", err)
	}
	if r.Error != nil {
		return "", fmt.Errorf("server error: %s", r.Error.Message)
	}
	var b strings.Builder
	for _, c := range r.Result.Content {
		b.WriteString(c.Text)
	}
	return b.String(), nil
}
