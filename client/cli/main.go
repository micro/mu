package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var version = "dev"

const help = `Mu CLI — connect to a Mu runtime

Usage: mu-cli [flags] COMMAND [arguments]

  login                 Sign in with your browser
  login --token-stdin   Save an API token read from standard input
  logout                Remove this computer's saved sign-in
  ask QUESTION          Ask once; use '-' to read from standard input
  chat                  Interactive chat (/new, /quit)
  agents                List available agents
  threads               List conversations
  read THREAD           Read a conversation
  operation ID          Recover an accepted request
  home | inbox | services
  call SERVICE ENDPOINT JSON   Make a MUCP call; '-' reads JSON from stdin
  version

Flags (before command):
  --server ORIGIN        Runtime (MU_URL, saved server, or https://micro.mu)
  --agent ID             Select an agent (default: Micro)
  --thread ID            Continue an existing conversation
  --page TOKEN           Read another page of results
  --message-id ID        Reuse an uncertain submission's ID (ask only)
  --timeout DURATION     Maximum time per command/chat turn (default: 10m)
  --json                Print the full response as JSON

MU_TOKEN supplies a token without saving it. No background polling occurs.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, safeText(err.Error()))
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, in io.Reader, out, diagnostics io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := io.WriteString(out, help)
		return err
	}
	if args[0] == "version" {
		fmt.Fprintln(out, version)
		return nil
	}
	path, err := sessionPath()
	if err != nil {
		return err
	}
	return runWithConfig(ctx, args, in, out, diagnostics, path)
}

func runWithConfig(ctx context.Context, args []string, in io.Reader, out, diagnostics io.Writer, path string) error {
	var err error
	server := os.Getenv("MU_URL")
	if server == "" {
		if s, e := readSession(path); e == nil {
			server = s.Server
		}
	}
	if server == "" {
		server = "https://micro.mu"
	}
	flags := flag.NewFlagSet("mu-cli", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.StringVar(&server, "server", server, "runtime origin")
	agent := flags.String("agent", "", "agent ID")
	thread := flags.String("thread", "", "thread ID")
	page := flags.String("page", "", "page token")
	messageID := flags.String("message-id", "", "submission ID")
	timeout := flags.Duration("timeout", 10*time.Minute, "command timeout")
	asJSON := flags.Bool("json", false, "JSON output")
	if err = flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if len(args) == 0 {
		return fmt.Errorf("provide a command; see mu-cli help")
	}
	if *timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	server, err = origin(server)
	if err != nil {
		return err
	}
	c := &client{server: server, token: os.Getenv("MU_TOKEN"), path: path, http: transport()}
	command := args[0]
	args = args[1:]
	if *messageID != "" && (command != "ask" || !regexp.MustCompile(`^[A-Za-z0-9_-]{16,120}$`).MatchString(*messageID)) {
		return fmt.Errorf("message-id must be 16–120 letters, digits, '-' or '_', and is only valid with ask")
	}
	if command == "login" || command == "logout" {
		if command == "logout" && len(args) != 0 {
			return fmt.Errorf("usage: mu-cli logout")
		}
		var s session
		if command == "login" {
			switch {
			case len(args) == 0:
				s, err = browserLogin(ctx, c.http, server, diagnostics, openBrowser)
			case len(args) == 1 && args[0] == "--token-stdin":
				var b []byte
				b, err = io.ReadAll(io.LimitReader(in, 16385))
				if err == nil {
					token := strings.TrimSpace(string(b))
					if token == "" || len(b) > 16384 || strings.ContainsAny(token, "\r\n\t ") {
						err = fmt.Errorf("provide one token of at most 16 KiB")
					} else {
						s = session{Server: server, Token: token}
					}
				}
			default:
				return fmt.Errorf("usage: mu-cli login [--token-stdin]")
			}
			if err != nil {
				return err
			}
		}
		unlock, err := lockSession(ctx, path)
		if err != nil {
			return err
		}
		defer unlock()
		if command == "logout" {
			err = os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
			if err == nil {
				fmt.Fprintln(diagnostics, "Local sign-in removed. Revoke the token on the server to end other uses.")
			}
			return err
		}
		if err = saveSession(path, s); err == nil {
			fmt.Fprintln(diagnostics, "Signed in to "+server)
		}
		return err
	}
	if command == "chat" {
		if len(args) != 0 {
			return fmt.Errorf("usage: mu-cli chat")
		}
		fmt.Fprintln(diagnostics, "Type a message. /new starts a chat; /quit exits.")
		chatCtx, stop := context.WithCancel(ctx)
		defer stop()
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 8192), 8192)
		type line struct {
			text string
			err  error
			eof  bool
		}
		lines := make(chan line)
		go func() {
			for scanner.Scan() {
				select {
				case lines <- line{text: scanner.Text()}:
				case <-chatCtx.Done():
					return
				}
			}
			select {
			case lines <- line{err: scanner.Err(), eof: true}:
			case <-chatCtx.Done():
			}
		}()
		for {
			fmt.Fprint(diagnostics, "> ")
			var input line
			select {
			case <-ctx.Done():
				return ctx.Err()
			case input = <-lines:
			}
			if input.eof {
				return input.err
			}
			prompt := strings.TrimSpace(input.text)
			if prompt == "/quit" {
				return nil
			}
			if prompt == "/new" {
				*thread = ""
				continue
			}
			if prompt == "" {
				continue
			}
			turn, cancel := context.WithTimeout(ctx, *timeout)
			next, e := ask(turn, c, *agent, *thread, prompt, "", *asJSON, out, diagnostics)
			cancel()
			if next != "" {
				*thread = next
			}
			if e != nil {
				return e
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if command == "ask" {
		if len(args) == 0 {
			return fmt.Errorf("usage: mu-cli ask QUESTION (or '-' for stdin)")
		}
		prompt := strings.Join(args, " ")
		if len(args) == 1 && args[0] == "-" {
			b, e := io.ReadAll(io.LimitReader(in, 8001))
			if e != nil {
				return e
			}
			prompt = string(b)
		}
		_, err = ask(ctx, c, *agent, *thread, prompt, *messageID, *asJSON, out, diagnostics)
		return err
	}
	body := map[string]any{}
	service, endpoint := "assistant", ""
	switch command {
	case "agents":
		endpoint = "Assistant.ListAgents"
	case "threads":
		endpoint = "Assistant.ListThreads"
		body = map[string]any{"agentId": *agent, "pageSize": 50, "pageToken": *page}
	case "read":
		if len(args) != 1 {
			return fmt.Errorf("usage: mu-cli read THREAD")
		}
		endpoint = "Assistant.ReadThread"
		body = map[string]any{"thread": args[0], "pageSize": 50, "pageToken": *page}
		args = nil
	case "home":
		endpoint = "Home.Overview"
	case "inbox":
		endpoint = "Inbox.List"
		body = map[string]any{"pageSize": 50, "pageToken": *page}
	case "services":
		endpoint = "Services.List"
	case "operation":
		if len(args) != 1 {
			return fmt.Errorf("usage: mu-cli operation ID")
		}
		var op operation
		if err = c.call(ctx, service, "Assistant.ReadOperation", map[string]string{"id": args[0]}, &op); err != nil {
			return err
		}
		if op.ID != args[0] {
			return fmt.Errorf("operation identity changed")
		}
		return finish(ctx, c, op, *asJSON, out, diagnostics)
	case "call":
		if len(args) != 3 {
			return fmt.Errorf("usage: mu-cli call SERVICE ENDPOINT JSON (or '-' for stdin)")
		}
		service, endpoint = args[0], args[1]
		raw := []byte(args[2])
		if args[2] == "-" {
			raw, err = io.ReadAll(io.LimitReader(in, 65537))
			if err != nil {
				return err
			}
		}
		if len(raw) > 65536 {
			return fmt.Errorf("request exceeds 64 KiB")
		}
		if err = json.Unmarshal(raw, &body); err != nil || body == nil {
			return fmt.Errorf("request must be a JSON object")
		}
		args = nil
	default:
		return fmt.Errorf("unknown command %q; see mu-cli help", command)
	}
	if len(args) != 0 {
		return fmt.Errorf("unexpected arguments for %s", command)
	}
	var result any
	if err = c.call(ctx, service, endpoint, body, &result); err != nil {
		return err
	}
	return printJSON(out, result)
}
func printJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func ask(ctx context.Context, c *client, agent, thread, prompt, id string, asJSON bool, out, diagnostics io.Writer) (string, error) {
	if !utf8.ValidString(prompt) || strings.TrimSpace(prompt) == "" || len(prompt) > 8000 {
		return thread, fmt.Errorf("write a message of 1–8,000 UTF-8 bytes")
	}
	if id == "" {
		id = randomID()
	}
	fmt.Fprintln(diagnostics, "Message ID: "+id)
	var op operation
	err := c.call(ctx, "assistant", "Assistant.Submit", map[string]any{"messageId": id, "request": map[string]any{"agentId": agent, "thread": thread, "prompt": prompt, "mode": "ACCESS_MODE_PERSONAL"}}, &op)
	if err != nil {
		return thread, fmt.Errorf("submission not confirmed: %w\nCheck Chats, or retry the same prompt, server, agent and thread with --message-id %s", err, id)
	}
	return op.Thread, finish(ctx, c, op, asJSON, out, diagnostics)
}
func finish(ctx context.Context, c *client, op operation, asJSON bool, out, diagnostics io.Writer) error {
	fmt.Fprintf(diagnostics, "Thread: %s\nOperation: %s\n", safeText(op.Thread), safeText(op.ID))
	result, err := c.wait(ctx, op)
	if err != nil {
		return fmt.Errorf("%w\nCheck the thread or resume with mu-cli --server %s operation %s", err, c.server, safeText(op.ID))
	}
	if asJSON {
		return printJSON(out, result)
	}
	if _, err = fmt.Fprintln(out, safeText(result.Result.Text)); err != nil {
		return err
	}
	for _, content := range result.Result.Content {
		if content.Fallback != "" {
			if _, err = fmt.Fprintln(out, safeText(content.Fallback)); err != nil {
				return err
			}
		}
	}
	return nil
}
