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

	"mu/agent/local"
	"mu/wallet"
)

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

	if !local.Configured() {
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

	// Say which provider, up front. A key can be set and still not answer — a
	// stale base URL replies 404 — and that failure used to arrive
	// mid-conversation with no clue which provider produced it.
	fmt.Printf("model: %s\n", local.Describe())

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

	// The one place that can spend money, handed to the loop as a function.
	// agent/local never learns what a wallet is.
	pay := func(ctx context.Context, name string, args map[string]any) (string, error) {
		return wallet.PayAndCallMCP(ctx, "", server, name, args, bw)
	}
	agent, err := local.New(local.Options{
		Tools:  asLocalTools(tools),
		Call:   pay,
		OnCall: func(name string) { fmt.Printf("· %s\n", name) },
	})
	if err != nil {
		fmt.Println("could not start:", err)
		return 1
	}

	// Yesterday's conversation, so the first question of the day is not asked
	// into a vacuum.
	if past := loadHistory(); len(past) > 0 {
		agent.Restore(past)
		fmt.Printf("history: %d earlier exchanges\n", len(past))
	}

	s := &session{bw: bw, agent: agent, opening: usdcRaw(bw)}

	if question != "" {
		fmt.Println()
		fmt.Println(s.ask(ctx, question))
		saveHistory(agent.History())
		s.reportSpend(s.opening)
		return 0
	}
	code := s.converse(ctx)
	saveHistory(agent.History())
	return code
}

// session is one run's state: the catalogue, the wallet and what has been said
// so far. Holding it across questions is what makes a second question cheap —
// the model can answer from what the first one already paid for.
type session struct {
	bw      *wallet.BaseWallet
	agent   *local.Session
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

// ask answers one question. The loop, the tool calling and the history are
// agent/local's; what belongs here is only how a call gets paid for.
func (s *session) ask(ctx context.Context, question string) string {
	answer, err := s.agent.Ask(ctx, question)
	if err != nil {
		return "the model failed: " + err.Error()
	}
	return answer
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

// asLocalTools converts the server's catalogue into what the loop needs. The
// JSON Schema is passed through untouched: it is the server's description of
// its own arguments, and the provider hands it to the model as-is.
func asLocalTools(tools []remoteTool) []local.Tool {
	out := make([]local.Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, local.Tool{Name: t.Name, Description: t.Description, Schema: t.Schema})
	}
	return out
}

// historyFile is where a conversation survives the process.
//
// Sessions were in memory only, so closing the terminal lost everything the
// last one had already paid to fetch — and the first question of the day
// started from nothing every day. A dotfile is the whole feature for now:
// enough that yesterday's answers are still there, small enough to delete.
func historyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".mu", "agent_history")
}

// loadHistory reads the last conversation, or nothing if there isn't one.
// Never an error: a missing or unreadable history is a session that starts
// fresh, which is exactly what happened before it existed.
func loadHistory() []local.Exchange {
	path := historyFile()
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var h []local.Exchange
	if err := json.Unmarshal(b, &h); err != nil {
		return nil
	}
	return h
}

// saveHistory writes the conversation back.
//
// 0600 because it holds whatever you asked and whatever came back — mail,
// messages, where you were going. It is as private as the questions were.
func saveHistory(h []local.Exchange) {
	path := historyFile()
	if path == "" || len(h) == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	b, err := json.Marshal(h)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o600)
}
