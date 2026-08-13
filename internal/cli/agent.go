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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
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

	// No question opens a conversation instead. One-shot is for scripts; a
	// session is for working, and it is the difference between paying to
	// rediscover context on every question and paying once.
	question := strings.TrimSpace(strings.Join(words, " "))

	// A name from X402_SERVERS resolves to its URL, so the instance you rent
	// from can be configured once rather than typed every time.
	if !strings.Contains(server, "://") {
		if u := wallet.ServerURL(server); u != "" {
			server = u
		}
	}

	if !ai.Configured() {
		fmt.Println("You need a model. This rents tools, not thinking — the tools")
		fmt.Println("come from the server, the thinking is yours.")
		fmt.Println()
		fmt.Println("Set one of these and run it again:")
		fmt.Println()
		fmt.Println("  export ANTHROPIC_API_KEY=sk-ant-...     # console.anthropic.com")
		fmt.Println("  export OPENROUTER_API_KEY=sk-or-...     # openrouter.ai/keys")
		fmt.Println("  export OPENAI_BASE_URL=http://localhost:11434/v1   # Ollama, or any")
		fmt.Println("                                                     # OpenAI-compatible server")
		return 1
	}

	// Say which model, up front. A key can be set and still not work — a stale
	// base URL answers 404 — and the failure then arrived mid-conversation as a
	// provider error with no clue which provider it came from. Naming it here
	// makes that a five-second diagnosis instead of a mystery.
	if status, ok := ai.Status(); !ok {
		fmt.Printf("model: %s\n", status)
		fmt.Println("       (that is what it will try; the first question may fail)")
	} else {
		fmt.Printf("model: %s\n", status)
	}

	ctx := context.Background()

	tools, err := fetchCatalogue(ctx, server)
	if err != nil {
		fmt.Println("could not read the catalogue:", err)
		return 1
	}
	fmt.Printf("%d tools from %s\n", len(tools), server)

	// The wallet is optional because much of the catalogue is free. Saying so
	// up front is better than failing on the first priced call, and an agent
	// that can answer from free tools alone should not be stopped at the door.
	bw := openOrCreateWallet(seedPath)
	s := &session{server: server, tools: tools, bw: bw, system: systemFor(tools)}
	s.opening = usdcRaw(bw)

	if question != "" {
		fmt.Println()
		fmt.Println(s.ask(ctx, question))
		s.reportSpend(s.opening)
		return 0
	}
	return s.converse(ctx)
}

// session is one run's state: the catalogue, the wallet and what has been said
// so far. Holding it across questions is what makes a second question cheap —
// the model can answer from what the first one already paid for.
type session struct {
	server  string
	tools   []remoteTool
	bw      *wallet.BaseWallet
	system  string
	history ai.History
	opening *big.Int
}

// converse reads questions until the input ends.
func (s *session) converse(ctx context.Context) int {
	fmt.Println()
	fmt.Println("Ask a question. Ctrl-D or \"exit\" to finish.")

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Print("\n> ")
		if !in.Scan() {
			break
		}
		q := strings.TrimSpace(in.Text())
		if q == "" {
			continue
		}
		if q == "exit" || q == "quit" {
			break
		}
		fmt.Println()
		fmt.Println(s.ask(ctx, q))
	}

	fmt.Println()
	s.reportSpend(s.opening)
	return 0
}

// reportSpend prints what has been spent since a balance was taken.
//
// Read from the chain rather than totted up from what we believe we
// authorised: the wallet is the one account of a run's cost that cannot be
// wrong, and an agent spending somebody's money should be checkable against
// something other than its own bookkeeping.
func (s *session) reportSpend(since *big.Int) {
	if since == nil || s.bw == nil {
		return
	}
	if spent := new(big.Int).Sub(since, usdcRaw(s.bw)); spent.Sign() > 0 {
		fmt.Printf("spent %s USDC\n", wallet.FormatUnits(spent, 6))
	}
}

// openOrCreateWallet loads the paying key, making one on a clean run.
//
// A first run used to print the path of a file that did not exist and leave it
// there — a dead end wearing the clothes of an error message. Generating a key
// is free, local and risks nothing (an empty wallet can lose nothing), so the
// only thing the old behaviour saved anybody was the step it did not tell them
// how to take.
//
// Returns nil only when there is genuinely no usable wallet, and the caller
// carries on regardless: most of the catalogue is free, and being unable to pay
// is not a reason to be unable to ask.
func openOrCreateWallet(seedPath string) *wallet.BaseWallet {
	bw, err := walletFromSeed(seedPath)
	if err == nil {
		human, raw := wallet.USDCBalance(bw.Address)
		if raw == nil || raw.Sign() == 0 {
			fmt.Printf("wallet: %s — empty\n", bw.Address)
			fmt.Println("        Send USDC on Base to it to use the priced tools.")
			fmt.Println("        Free tools work now; a priced one will say it could not pay.")
			return bw
		}
		fmt.Printf("wallet: %s (%s USDC)\n", bw.Address, human)
		return bw
	}

	// Anything other than "it is not there yet" is a real problem with a real
	// file, and quietly making a second key beside it would be worse than
	// saying so.
	path := seedPath
	if path == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			fmt.Println("wallet: no wallet, and no home directory to put one in —", herr)
			return nil
		}
		path = filepath.Join(home, ".mu", "keys", "wallet.seed")
	}
	if _, serr := os.Stat(path); serr == nil {
		fmt.Printf("wallet: %s could not be used (%v) — free tools only\n", path, err)
		return nil
	}

	addr, cerr := createSeed(path)
	if cerr != nil {
		fmt.Println("wallet: could not create one —", cerr)
		fmt.Println("        Free tools will still answer.")
		return nil
	}
	fmt.Printf("wallet: created %s\n", addr)
	fmt.Printf("        key at %s — back it up, it is the only copy\n", path)
	fmt.Println()
	fmt.Println("        You need to fund this wallet to use the priced tools.")
	fmt.Println("        Send USDC on Base to the address above. No ETH needed —")
	fmt.Println("        payments are signed here and somebody else pays the gas.")

	bw, err = walletFromSeed(path)
	if err != nil {
		return nil
	}
	return bw
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

// ask runs the model/tool loop until the model stops asking for tools, and
// keeps the exchange so the next question can build on it.
func (s *session) ask(ctx context.Context, question string) string {
	turn := s.history
	prompt := question

	for step := 0; step < maxSteps; step++ {
		reply, err := ai.Ask(&ai.Prompt{
			System:   s.system,
			Question: prompt,
			Context:  turn,
			Caller:   "cli_agent",
		})
		if err != nil {
			return "the model failed: " + err.Error()
		}

		call, ok := parseToolCall(reply)
		if !ok {
			answer := strings.TrimSpace(reply)
			s.history = trimHistory(append(turn, ai.Message{Prompt: prompt, Answer: answer}))
			return answer
		}

		fmt.Printf("· %s\n", call.Tool)
		out, err := wallet.PayAndCallMCP(ctx, "", s.server, call.Tool, call.Args, s.bw)
		if err != nil {
			// A failed call is told to the model rather than to the user. It
			// may well have asked for a tool that needs an account, and the
			// useful next move is for it to try another one.
			out = "that call failed: " + err.Error()
		}

		// The model's own request and the result go back as one turn, so the
		// next pass can see what it asked for as well as what it got.
		turn = append(turn, ai.Message{Prompt: prompt, Answer: reply})
		prompt = "Result of " + call.Tool + ":\n" + truncate(out, 6000) +
			"\n\nAnswer the original question, or call another tool."
	}

	s.history = trimHistory(turn)
	return "gave up after " + fmt.Sprint(maxSteps) + " steps without settling on an answer"
}

// historyLimit caps what is carried between questions. Tool results are
// verbose, every message is re-sent on the next call, and a session left open
// all afternoon would otherwise grow until the model refuses it — or until
// each question costs more than the tool it calls.
const historyLimit = 20

func trimHistory(h ai.History) ai.History {
	if len(h) <= historyLimit {
		return h
	}
	return append(ai.History{}, h[len(h)-historyLimit:]...)
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
