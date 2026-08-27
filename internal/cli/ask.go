package cli

// `mu ask` — talking to your agent from a terminal.
//
// The gap this fills is easy to miss, because the binary already has a command
// with "agent" in the name. `mu agent` is the opposite thing: an agent that
// runs on your machine, brings your model key, and rents tools off an instance
// over x402 — see agent.go. It never touches the agent you made, its standing
// instruction, its scope or its memory.
//
// So there was no way to reach your own agent from a terminal at all. The web
// page could, mail could, a program with curl could — and the command line,
// which is the surface a developer is already in, could not. What was there
// instead was two dead entries in defaultArgKey mapping `chat` and `agent` to a
// prompt parameter: `chat` is the discussion-rooms service now and has no such
// field, and `agent` stopped being a tool when agent_ask was removed.
//
// # It is the same door a program uses
//
// POST /agent/<name>, with the account's token — see agent/api.go. Not /mcp,
// which is the tools this agent may use rather than the agent, and not a new
// endpoint, because the address of an agent is the address of an agent.
//
// That means the conversation is written to the record like any other: it turns
// up at /inbox and /recall, it continues in a browser, and an answer you got
// here is one the agent remembers next time. A terminal client that kept its
// own history in a file would be the fifth private copy of a conversation, and
// there is a section of AGENTS.md about the last four.
//
// # The thread is the whole point
//
//	mu ask "what is on my calendar"
//	mu ask                              # a prompt, and it keeps the thread
//
// One-shot opens a conversation and prints the answer. Interactive keeps the
// thread id the server returns and passes it back, which is the difference
// between a conversation and a series of unrelated questions.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// runAsk handles `mu ask [--agent name] [question]`.
func runAsk(args []string, rc *ResolvedConfig) int {
	agent := ""
	var words []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--agent" && i+1 < len(args):
			agent, i = args[i+1], i+1
		case strings.HasPrefix(args[i], "--agent="):
			agent = strings.TrimPrefix(args[i], "--agent=")
		default:
			words = append(words, args[i])
		}
	}
	if err := rc.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	// The default agent, named rather than left blank, so the URL is a real
	// address somebody can read back off the screen and paste into curl.
	if strings.TrimSpace(agent) == "" {
		agent = defaultAgent
	}
	// A slug, not a path. Without this `--agent ../../admin` would be a URL
	// this client built out of an argument, and the fact that the server would
	// refuse it is not a reason to send it.
	if strings.ContainsAny(agent, "/?#") {
		fmt.Fprintf(os.Stderr, "an agent is named, not a path: %s\n", agent)
		return 2
	}

	a := &asker{
		url:   strings.TrimRight(rc.URL, "/") + "/agent/" + agent,
		token: rc.Token,
		http:  &http.Client{Timeout: 180 * time.Second},
	}

	if len(words) > 0 {
		text, err := a.ask(strings.Join(words, " "))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(text)
		return 0
	}
	return a.loop(agent)
}

// defaultAgent is the one every instance provides. Duplicated from
// agent.DefaultSlug rather than imported, because this file is the thin client
// half of the binary and the value is part of the wire contract — the same
// reason DefaultURL is a constant here.
const defaultAgent = "micro"

// asker holds one conversation.
type asker struct {
	url   string
	token string
	http  *http.Client

	// thread is what the server called this conversation. Empty on the first
	// call; sent on every one after it, which is what makes the second question
	// a follow-up rather than a stranger.
	thread string
}

// ask sends one question and returns the answer, remembering the thread.
func (a *asker) ask(text string) (string, error) {
	body, err := json.Marshal(map[string]string{"text": text, "thread": a.thread})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", a.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request to %s: %w", a.url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// The failures worth naming, said the way the rest of this client says
	// them — see Client.call, which answers the same three.
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return "", fmt.Errorf("not signed in — run `mu login` or set MU_TOKEN")
	case resp.StatusCode == http.StatusPaymentRequired:
		return "", fmt.Errorf("out of credits — top up at %s",
			strings.SplitN(a.url, "/agent/", 2)[0]+"/wallet/topup")
	case resp.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("no agent at %s", a.url)
	case resp.StatusCode >= 400:
		if msg := httpErrorMessage(raw); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, a.url)
	}

	var out struct {
		Text   string `json:"text"`
		Thread string `json:"thread"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("could not read the answer: %w", err)
	}
	if out.Thread != "" {
		a.thread = out.Thread
	}
	return out.Text, nil
}

// loop is the prompt.
//
// Deliberately not a terminal UI: no cursor addressing, no colour, no
// alternate screen. This runs down a pipe and inside a container as often as it
// runs on somebody's laptop, and every one of those tricks is a way for it to
// look wrong in one of them.
func (a *asker) loop(agent string) int {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64<<10), 1<<20)

	// Said once, at the top, because somebody who typed `mu ask` and got a bare
	// cursor has no way to know what this is waiting for.
	fmt.Printf("Talking to %s. A blank line or Ctrl-D ends it.\n", agent)
	for {
		fmt.Print("> ")
		if !in.Scan() {
			fmt.Println()
			return 0
		}
		text := strings.TrimSpace(in.Text())
		if text == "" {
			return 0
		}
		answer, err := a.ask(text)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			// Not fatal. A question that failed — a timeout, a credit that ran
			// out mid-conversation — should not throw away the thread and the
			// conversation with it.
			continue
		}
		fmt.Println(answer)
	}
}
