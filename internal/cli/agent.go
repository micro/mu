package cli

// `mu agent` — an agent that owns a wallet and rents its tools.
//
// Every other mode of this binary assumes an instance: `mu --serve` runs one,
// and the tool subcommands call one you are signed into. This mode assumes
// none. It holds a model key and a private key, and it gets its capabilities
// from somebody else's instance, paying per call over x402.
//
// That split is the point. A catalogue is free to read — `tools/list` needs no
// account and no payment — so an agent can discover ninety-six tools, decide
// which one answers the question, and only then spend anything. Discovery
// being free is what makes renting tools practical rather than a subscription
// with extra steps.
//
//	mu agent "what happened in markets today?"
//	mu agent --server https://micro.mu "book me somewhere for lunch near EC2"
//
// What it does not rent is the thinking. There is no inference tool in the
// catalogue, deliberately — a model is the one thing every developer already
// has, and the thirty provider accounts are the part worth not having. So this
// brings your own model (Anthropic, OpenRouter, or Ollama on your own machine)
// and buys everything else two pence at a time.

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

	"mu/internal/ai"
	"mu/wallet"
)

// maxSteps bounds a run. A model that keeps calling tools without converging
// is spending real money on every pass, so the loop has to end on its own
// rather than when somebody notices.
const maxSteps = 8

// remoteTool is one entry from the server's catalogue.
type remoteTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"inputSchema"`
}

// runAgent handles `mu agent <question>`.
func runAgent(args []string) int {
	server := "https://micro.mu"
	seedPath := ""
	var words []string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--server" && i+1 < len(args):
			server, i = args[i+1], i+1
		case strings.HasPrefix(args[i], "--server="):
			server = strings.TrimPrefix(args[i], "--server=")
		case args[i] == "--seed" && i+1 < len(args):
			seedPath, i = args[i+1], i+1
		case strings.HasPrefix(args[i], "--seed="):
			seedPath = strings.TrimPrefix(args[i], "--seed=")
		default:
			words = append(words, args[i])
		}
	}

	question := strings.TrimSpace(strings.Join(words, " "))
	if question == "" {
		fmt.Println("usage: mu agent [--server URL] [--seed PATH] <question>")
		fmt.Println()
		fmt.Println("  An agent with your model and your wallet, using another")
		fmt.Println("  instance's tools and paying per call.")
		fmt.Println()
		fmt.Println("  mu agent \"what happened in markets today?\"")
		return 2
	}

	// A name from X402_SERVERS resolves to its URL, so the instance you rent
	// from can be configured once rather than typed every time.
	if !strings.Contains(server, "://") {
		if u := wallet.ServerURL(server); u != "" {
			server = u
		}
	}

	if !ai.Configured() {
		fmt.Println("No model configured. This mode rents tools, not thinking —")
		fmt.Println("set ANTHROPIC_API_KEY, OPENROUTER_API_KEY, or run a local")
		fmt.Println("model and set LOCAL_MODEL_URL.")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tools, err := fetchCatalogue(ctx, server)
	if err != nil {
		fmt.Println("could not read the catalogue:", err)
		return 1
	}
	fmt.Printf("%d tools from %s\n", len(tools), server)

	// The wallet is optional because much of the catalogue is free. Saying so
	// up front is better than failing on the first priced call, and an agent
	// that can answer from free tools alone should not be stopped at the door.
	bw, err := walletFromSeed(seedPath)
	if err != nil {
		fmt.Printf("no wallet (%v) — free tools only\n", err)
	} else {
		human, _ := wallet.USDCBalance(bw.Address)
		fmt.Printf("paying from %s (%s USDC)\n", bw.Address, human)
	}
	before := usdcRaw(bw)

	answer := think(ctx, server, question, tools, bw)

	// Spend is reported from the chain rather than from what we believe we
	// authorised. The wallet is the only account of what a run cost that
	// cannot be wrong.
	if spent := new(big.Int).Sub(before, usdcRaw(bw)); spent.Sign() > 0 {
		fmt.Printf("\nspent %s USDC\n", wallet.FormatUnits(spent, 6))
	}
	fmt.Println()
	fmt.Println(answer)
	return 0
}

// usdcRaw reads the wallet's balance in atomic units, or zero when there is no
// wallet to read.
func usdcRaw(bw *wallet.BaseWallet) *big.Int {
	if bw == nil {
		return big.NewInt(0)
	}
	_, raw := wallet.USDCBalance(bw.Address)
	if raw == nil {
		return big.NewInt(0)
	}
	return raw
}

// think runs the model/tool loop until the model stops asking for tools.
func think(ctx context.Context, server, question string, tools []remoteTool, bw *wallet.BaseWallet) string {
	system := systemFor(tools)
	history := ai.History{}
	ask := question

	for step := 0; step < maxSteps; step++ {
		reply, err := ai.Ask(&ai.Prompt{
			System:   system,
			Question: ask,
			Context:  history,
			Caller:   "cli_agent",
		})
		if err != nil {
			return "the model failed: " + err.Error()
		}

		call, ok := parseToolCall(reply)
		if !ok {
			return strings.TrimSpace(reply)
		}

		fmt.Printf("· %s\n", call.Tool)
		out, err := wallet.PayAndCallMCP(ctx, "", server, call.Tool, call.Args, bw)
		if err != nil {
			out = "that call failed: " + err.Error()
		}

		// The model's own request and the result go back as one turn, so the
		// next pass can see what it asked for as well as what it got.
		history = append(history, ai.Message{Prompt: ask, Answer: reply})
		ask = "Result of " + call.Tool + ":\n" + truncate(out, 6000) +
			"\n\nAnswer the original question, or call another tool."
	}
	return "gave up after " + fmt.Sprint(maxSteps) + " steps without settling on an answer"
}

// toolCall is the model's request to use a tool.
type toolCall struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

// parseToolCall reads a tool request out of a model reply, tolerating the code
// fence models habitually wrap JSON in. Anything that is not a well-formed
// call naming a tool is prose, which is how the model says it has finished.
func parseToolCall(reply string) (toolCall, bool) {
	s := strings.TrimSpace(reply)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimPrefix(s, "json")
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
	}
	if !strings.HasPrefix(s, "{") {
		return toolCall{}, false
	}
	var c toolCall
	if err := json.Unmarshal([]byte(s), &c); err != nil || c.Tool == "" {
		return toolCall{}, false
	}
	if c.Args == nil {
		c.Args = map[string]any{}
	}
	return c, true
}

// systemFor renders the catalogue into the system prompt.
func systemFor(tools []remoteTool) string {
	var b strings.Builder
	b.WriteString("You are an agent with access to tools on a remote server. ")
	b.WriteString("Today's date is " + time.Now().Format("2 January 2006") + ".\n\n")
	b.WriteString("To use a tool, reply with ONLY a JSON object and nothing else:\n")
	b.WriteString(`{"tool":"tool_name","args":{"key":"value"}}` + "\n\n")
	b.WriteString("When you can answer, reply with prose and no JSON. Prefer answering ")
	b.WriteString("from what you already know; call a tool when the question needs ")
	b.WriteString("current data. Each call costs the user real money, so do not call ")
	b.WriteString("a tool twice for the same thing.\n\nTools:\n")

	for _, t := range tools {
		b.WriteString("- " + t.Name)
		if d := strings.TrimSpace(t.Description); d != "" {
			b.WriteString(": " + truncate(d, 160))
		}
		if params := paramNames(t.Schema); params != "" {
			b.WriteString(" [" + params + "]")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// paramNames renders a tool's parameters as "name:type" pairs, with required
// ones marked. The full JSON Schema for ninety-six tools would be most of a
// context window and almost none of it load-bearing.
func paramNames(schema map[string]any) string {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return ""
	}
	required := map[string]bool{}
	if list, ok := schema["required"].([]any); ok {
		for _, r := range list {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}
	var out []string
	for name, raw := range props {
		kind := "string"
		if m, ok := raw.(map[string]any); ok {
			if t, ok := m["type"].(string); ok {
				kind = t
			}
		}
		entry := name + ":" + kind
		if required[name] {
			entry += "*"
		}
		out = append(out, entry)
	}
	return strings.Join(out, " ")
}

// fetchCatalogue reads the server's tool list, which costs nothing and needs
// no credentials — the property this whole mode is built on.
func fetchCatalogue(ctx context.Context, server string) ([]remoteTool, error) {
	body, err := mcpPost(ctx, strings.TrimRight(server, "/")+"/mcp", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list",
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			Tools []remoteTool `json:"tools"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("%s", resp.Error.Message)
	}
	if len(resp.Result.Tools) == 0 {
		return nil, fmt.Errorf("%s advertises no tools", server)
	}
	return resp.Result.Tools, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// mcpPost sends a JSON-RPC request to an MCP endpoint and returns the body.
//
// Only used for tools/list, which is free and unauthenticated. Anything that
// might be charged for goes through wallet.PayAndCallMCP instead, so there is
// exactly one path in this file that can spend money.
func mcpPost(ctx context.Context, endpoint string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
