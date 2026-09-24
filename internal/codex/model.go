package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	gmai "go-micro.dev/v6/model"
	"mu/internal/auth"
)

func init() {
	gmai.Register("codex", func(o ...gmai.Option) gmai.Model { return &provider{opts: gmai.NewOptions(o...)} })
}

type provider struct{ opts gmai.Options }

func (p *provider) Init(o ...gmai.Option) error {
	for _, f := range o {
		f(&p.opts)
	}
	return nil
}
func (p *provider) Options() gmai.Options { return p.opts }
func (p *provider) String() string        { return "codex" }
func (p *provider) Stream(context.Context, *gmai.Request, ...gmai.GenerateOption) (gmai.Stream, error) {
	return nil, gmai.ErrStreamingUnsupported
}

type streamKey struct{}
type accountKey struct{}

// ForAccount authorizes only the opted-in administrator, never a provider name
// received from a client. Generate rechecks the preference before dispatch.
func ForAccount(ctx context.Context, id string) (context.Context, error) {
	if !auth.CodexPreviewEnabled(id) {
		return ctx, fmt.Errorf("Codex preview is not enabled for this account")
	}
	return context.WithValue(ctx, accountKey{}, id), nil
}

type hooks struct {
	start func()
	token func(string)
}

// Live keeps token delivery on the same model/tool loop as other providers.
func Live(ctx context.Context, start func(), token func(string)) context.Context {
	return context.WithValue(ctx, streamKey{}, hooks{start, token})
}

func (p *provider) Generate(ctx context.Context, req *gmai.Request, _ ...gmai.GenerateOption) (*gmai.Response, error) {
	id, _ := ctx.Value(accountKey{}).(string)
	if !auth.CodexPreviewEnabled(id) {
		return nil, fmt.Errorf("Codex preview requires an opted-in admin account")
	}
	if !Checked() {
		return nil, fmt.Errorf("Codex preview is not checked; run mu codex check on this host")
	}
	c, close, e := start(ctx)
	if e != nil {
		return nil, e
	}
	defer close()
	return p.generate(ctx, c, req)
}

func (p *provider) generate(ctx context.Context, c *rpc, req *gmai.Request) (*gmai.Response, error) {
	if e := c.initialize(); e != nil {
		return nil, e
	}
	var account struct {
		Account *struct {
			Type string `json:"type"`
		} `json:"account"`
	}
	if e := c.call("account/read", map[string]bool{"refreshToken": false}, &account); e != nil {
		return nil, e
	}
	if account.Account == nil || account.Account.Type != "chatgpt" {
		return nil, fmt.Errorf("Micro's Codex profile is not signed in with ChatGPT; run mu codex login")
	}
	tools := make([]map[string]any, 0, len(req.Tools))
	allowed := map[string]bool{}
	for _, t := range req.Tools {
		if allowed[t.Name] {
			return nil, fmt.Errorf("duplicate Codex tool name")
		}
		allowed[t.Name] = true
		tools = append(tools, map[string]any{"type": "function", "name": t.Name, "description": t.Description, "inputSchema": map[string]any{"type": "object", "properties": t.Properties}})
	}
	model := p.opts.Model
	if model == "" {
		model = Model
	}
	var thread struct {
		Thread struct {
			ID        string `json:"id"`
			Ephemeral bool   `json:"ephemeral"`
		} `json:"thread"`
		Model              string   `json:"model"`
		InstructionSources []string `json:"instructionSources"`
	}
	if e := c.call("thread/start", map[string]any{"model": model, "allowProviderModelFallback": false, "ephemeral": true, "cwd": "/work", "environments": []any{}, "selectedCapabilityRoots": []any{}, "runtimeWorkspaceRoots": []any{}, "approvalPolicy": "never", "sandbox": "read-only", "baseInstructions": req.SystemPrompt, "developerInstructions": "Use only the supplied Micro tools. You have no access to ChatGPT history, personal connectors or host files.", "dynamicTools": tools}, &thread); e != nil {
		return nil, e
	}
	if thread.Thread.ID == "" || !thread.Thread.Ephemeral || thread.Model != model || len(thread.InstructionSources) != 0 {
		return nil, fmt.Errorf("Codex did not establish the requested isolated session")
	}
	// Preserve roles. Never flatten untrusted history into the system prompt.
	items := []any{}
	for _, m := range req.Messages {
		text, ok := m.Content.(string)
		if !ok {
			return nil, fmt.Errorf("Codex preview currently supports text history only")
		}
		if m.Role != "user" && m.Role != "assistant" && m.Role != "system" {
			return nil, fmt.Errorf("unsupported Codex history role")
		}
		kind := "input_text"
		if m.Role == "assistant" {
			kind = "output_text"
		}
		items = append(items, map[string]any{"type": "message", "role": m.Role, "content": []any{map[string]string{"type": kind, "text": text}}})
	}
	if len(items) > 0 {
		if e := c.call("thread/inject_items", map[string]any{"threadId": thread.Thread.ID, "items": items}, nil); e != nil {
			return nil, e
		}
	}
	res := &gmai.Response{}
	done := false
	turnID := ""
	seen := map[string]bool{}
	var final []string
	live, _ := ctx.Value(streamKey{}).(hooks)
	streamStarted := false
	c.handle = func(m message) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		var ev struct {
			ThreadID  string         `json:"threadId"`
			TurnID    string         `json:"turnId"`
			CallID    string         `json:"callId"`
			Tool      string         `json:"tool"`
			Namespace string         `json:"namespace"`
			Arguments map[string]any `json:"arguments"`
			Delta     string         `json:"delta"`
			Item      struct {
				ID    string `json:"id"`
				Type  string `json:"type"`
				Text  string `json:"text"`
				Phase string `json:"phase"`
			} `json:"item"`
			Turn struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"turn"`
			TokenUsage struct {
				Total struct {
					Input  int `json:"inputTokens"`
					Output int `json:"outputTokens"`
					Total  int `json:"totalTokens"`
				} `json:"total"`
			} `json:"tokenUsage"`
		}
		if e := json.Unmarshal(m.Params, &ev); e != nil {
			return fmt.Errorf("invalid Codex event")
		}
		if ev.ThreadID != "" && ev.ThreadID != thread.Thread.ID {
			return fmt.Errorf("Codex event belongs to another session")
		}
		if ev.TurnID != "" && turnID != "" && ev.TurnID != turnID {
			return fmt.Errorf("Codex event belongs to another turn")
		}
		if len(m.ID) > 0 {
			if m.Method != "item/tool/call" || ev.ThreadID != thread.Thread.ID || ev.TurnID == "" || ev.Namespace != "" || !allowed[ev.Tool] || ev.CallID == "" || p.opts.ToolHandler == nil {
				return fmt.Errorf("Codex requested a capability outside this user's tool scope")
			}
			if seen[ev.CallID] {
				return fmt.Errorf("duplicate Codex tool call")
			}
			seen[ev.CallID] = true
			if len(seen) > 100 {
				return fmt.Errorf("Codex tool limit reached")
			}
			call := gmai.ToolCall{ID: ev.CallID, Name: ev.Tool, Input: ev.Arguments}
			result := p.opts.ToolHandler(ctx, call)
			call.Result = result.Content
			res.ToolCalls = append(res.ToolCalls, call)
			if e := ctx.Err(); e != nil {
				return e
			}
			if e := c.out.Encode(map[string]any{"id": m.ID, "result": map[string]any{"success": result.Refused == "", "contentItems": []any{map[string]string{"type": "inputText", "text": result.Content}}}}); e != nil {
				return e
			}
			if result.Refused != "" {
				return fmt.Errorf("Codex tool call was refused (%s)", result.Refused)
			}
			return nil
		}
		switch m.Method {
		case "turn/started":
			if ev.ThreadID != thread.Thread.ID || ev.Turn.ID == "" || (turnID != "" && turnID != ev.Turn.ID) {
				return fmt.Errorf("invalid Codex turn")
			}
			turnID = ev.Turn.ID
		case "item/started", "item/completed":
			if ev.ThreadID != thread.Thread.ID || ev.TurnID == "" {
				return fmt.Errorf("unscoped Codex item")
			}
			switch ev.Item.Type {
			case "userMessage", "agentMessage", "reasoning", "dynamicToolCall", "contextCompaction":
			default:
				return fmt.Errorf("Codex exposed an unexpected built-in capability")
			}
			if m.Method == "item/completed" && ev.Item.Type == "agentMessage" && ev.Item.Phase != "commentary" {
				final = append(final, ev.Item.Text)
			}
		case "item/agentMessage/delta":
			if ev.ThreadID != thread.Thread.ID || ev.TurnID == "" {
				return fmt.Errorf("unscoped Codex message")
			}
			if !streamStarted {
				streamStarted = true
				if live.start != nil {
					live.start()
				}
			}
			if live.token != nil {
				live.token(ev.Delta)
			}
		case "thread/tokenUsage/updated":
			res.Usage = gmai.Usage{InputTokens: ev.TokenUsage.Total.Input, OutputTokens: ev.TokenUsage.Total.Output, TotalTokens: ev.TokenUsage.Total.Total}
		case "turn/completed":
			if ev.ThreadID != thread.Thread.ID || ev.Turn.ID == "" || (turnID != "" && ev.Turn.ID != turnID) {
				return fmt.Errorf("invalid Codex completion")
			}
			if ev.Turn.Status != "completed" {
				return fmt.Errorf("Codex turn did not complete (%s)", ev.Turn.Status)
			}
			done = true
		case "error":
			return fmt.Errorf("Codex reported a turn error")
		}
		return nil
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	e := c.call("turn/start", map[string]any{"threadId": thread.Thread.ID, "input": []any{map[string]any{"type": "text", "text": req.Prompt, "text_elements": []any{}}}, "environments": []any{}, "approvalPolicy": "never", "sandboxPolicy": map[string]any{"type": "readOnly", "access": map[string]any{"type": "restricted", "includePlatformDefaults": false, "readableRoots": []any{}}}}, &turn)
	if e != nil {
		return nil, e
	}
	if turn.Turn.ID == "" || (turnID != "" && turnID != turn.Turn.ID) {
		return nil, fmt.Errorf("invalid Codex turn acknowledgement")
	}
	turnID = turn.Turn.ID
	for !done {
		m, e := c.read()
		if e != nil {
			return nil, e
		}
		if e = c.dispatch(m); e != nil {
			return nil, e
		}
	}
	res.Reply = strings.Join(final, "\n\n")
	res.Answer = res.Reply
	if strings.TrimSpace(res.Reply) == "" {
		return nil, fmt.Errorf("Codex returned no final answer")
	}
	return res, nil
}
