package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	gmai "go-micro.dev/v6/model"
)

const prefix = `{"id":1,"result":{}}
{"id":2,"result":{"account":{"type":"chatgpt","email":"not-model-context@example.com"}}}
{"id":3,"result":{"thread":{"id":"mine","ephemeral":true},"model":"gpt-6-astra","instructionSources":[]}}
`
const turn = `{"id":4,"result":{"turn":{"id":"turn1"}}}
`
const completion = `{"method":"item/completed","params":{"threadId":"mine","turnId":"turn1","item":{"id":"answer","type":"agentMessage","text":"Done","phase":"final_answer"}}}
{"method":"thread/tokenUsage/updated","params":{"threadId":"mine","turnId":"turn1","tokenUsage":{"total":{"inputTokens":10,"outputTokens":2,"totalTokens":12}}}}
{"method":"turn/completed","params":{"threadId":"mine","turn":{"id":"turn1","status":"completed"}}}
`
const call = `{"id":"server1","method":"item/tool/call","params":{"threadId":"mine","turnId":"turn1","callId":"call1","tool":"notes_read","arguments":{"id":"note"}}}
`

func TestScopedToolsAndFreshThread(t *testing.T) {
	for _, secret := range []string{"alice-only", "bob-only"} {
		var out bytes.Buffer
		calls := 0
		p := provider{opts: gmai.NewOptions(gmai.WithToolHandler(func(ctx context.Context, c gmai.ToolCall) gmai.ToolResult {
			calls++
			return gmai.ToolResult{ID: c.ID, Content: secret}
		}))}
		res, e := p.generate(context.Background(), newRPC(strings.NewReader(prefix+turn+call+completion), &out), &gmai.Request{Prompt: secret, Tools: []gmai.Tool{{Name: "notes_read", Properties: map[string]any{"id": map[string]any{"type": "string"}}}}})
		if e != nil || res.Reply != "Done" || res.Usage.TotalTokens != 12 || calls != 1 {
			t.Fatalf("result=%+v error=%v calls=%d", res, e, calls)
		}
		if strings.Contains(out.String(), "not-model-context") || strings.Contains(out.String(), "thread/resume") {
			t.Fatal("account metadata or existing sessions were exposed")
		}
		dec := json.NewDecoder(&out)
		for dec.More() {
			var req map[string]any
			if e = dec.Decode(&req); e != nil {
				t.Fatal(e)
			}
			if req["method"] == "thread/start" {
				p := req["params"].(map[string]any)
				if p["ephemeral"] != true || len(p["environments"].([]any)) != 0 || p["allowProviderModelFallback"] != false {
					t.Fatal("session isolation missing")
				}
				if len(p["dynamicTools"].([]any)) != 1 {
					t.Fatal("tool catalogue broadened")
				}
			}
		}
		other := "alice-only"
		if secret == other {
			other = "bob-only"
		}
		if strings.Contains(out.String(), other) {
			t.Fatal("cross-account context")
		}
	}
}

func TestRejectsUnexpectedCapabilitiesAndSessionEvents(t *testing.T) {
	for name, event := range map[string]string{
		"other session":     strings.Replace(call, `"mine"`, `"someone-else"`, 1),
		"other turn":        strings.Replace(call, `"turn1"`, `"other-turn"`, 1),
		"unadvertised tool": strings.Replace(call, "notes_read", "shell_run", 1),
		"approval":          strings.Replace(call, "item/tool/call", "item/commandExecution/requestApproval", 1),
		"native shell":      `{"method":"item/started","params":{"threadId":"mine","turnId":"turn1","item":{"type":"commandExecution"}}}` + "\n",
		"foreign delta":     `{"method":"item/agentMessage/delta","params":{"threadId":"theirs","turnId":"turn1","delta":"PRIVATE"}}` + "\n",
		"unscoped delta":    `{"method":"item/agentMessage/delta","params":{"delta":"PRIVATE"}}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			called := false
			stream := ""
			p := provider{opts: gmai.NewOptions(gmai.WithToolHandler(func(context.Context, gmai.ToolCall) gmai.ToolResult { called = true; return gmai.ToolResult{} }))}
			ctx := Live(context.Background(), nil, func(s string) { stream += s })
			_, e := p.generate(ctx, newRPC(strings.NewReader(prefix+turn+event+completion), &out), &gmai.Request{Prompt: "test", Tools: []gmai.Tool{{Name: "notes_read"}}})
			if e == nil || called || stream != "" {
				t.Fatalf("unsafe event accepted: %v calls=%t stream=%q", e, called, stream)
			}
		})
	}
}

func TestHistoryRolesAndNoSessionReuse(t *testing.T) {
	var out bytes.Buffer
	wire := prefix + `{"id":4,"result":{}}` + "\n" + strings.Replace(turn, `"id":4`, `"id":5`, 1) + completion
	p := provider{}
	_, e := p.generate(context.Background(), newRPC(strings.NewReader(wire), &out), &gmai.Request{Prompt: "now", Messages: []gmai.Message{{Role: "user", Content: "private earlier message"}, {Role: "assistant", Content: "earlier answer"}}})
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), `"role":"assistant"`) || !strings.Contains(out.String(), `"type":"output_text"`) || strings.Contains(out.String(), "thread/list") {
		t.Fatal("history roles or session isolation lost")
	}
}

func TestRefusalCancellationAndDuplicateDoNotRetry(t *testing.T) {
	for _, kind := range []string{"refusal", "duplicate", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			p := provider{opts: gmai.NewOptions(gmai.WithToolHandler(func(context.Context, gmai.ToolCall) gmai.ToolResult {
				calls++
				r := gmai.ToolResult{}
				if kind == "refusal" {
					r.Refused = "not_permitted"
				}
				if kind == "cancel" {
					cancel()
				}
				return r
			}))}
			var out bytes.Buffer
			_, e := p.generate(ctx, newRPC(strings.NewReader(prefix+turn+call+call+completion), &out), &gmai.Request{Tools: []gmai.Tool{{Name: "notes_read"}}})
			if e == nil || calls != 1 {
				t.Fatalf("error=%v calls=%d", e, calls)
			}
		})
	}
}

func TestProviderErrorsDoNotExposeSecrets(t *testing.T) {
	var out bytes.Buffer
	c := newRPC(strings.NewReader(`{"id":1,"error":{"code":401,"message":"SECRET-TOKEN"}}`), &out)
	e := c.initialize()
	if e == nil || strings.Contains(e.Error(), "SECRET") {
		t.Fatal("provider error leaked")
	}
}

func TestProviderNameDoesNotGrantPreviewAccess(t *testing.T) {
	p := gmai.New("codex")
	_, e := p.Generate(context.Background(), &gmai.Request{Prompt: "read other conversations"})
	if e == nil || !strings.Contains(e.Error(), "opted-in admin") {
		t.Fatalf("provider bypassed account gate: %v", e)
	}
}

func TestToolArraysHaveItemsWithoutChangingRegistry(t *testing.T) {
	original := map[string]any{"type": "array"}
	got := toolSchema(original).(map[string]any)
	if got["items"] == nil || original["items"] != nil {
		t.Fatal("array adaptation missing or registry mutated")
	}
}
