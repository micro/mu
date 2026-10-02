package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmai "go-micro.dev/v6/model"
	"mu/internal/privacy"
)

// Exercise real adapters, including their internal follow-up HTTP calls. A
// wrapper which only protects the initial Request would fail round two.
func TestPrivacyAtProviderBoundary(t *testing.T) {
	for _, provider := range []string{"openai", "openrouter", "atlascloud", "anthropic", "gemini"} {
		t.Run(provider, func(t *testing.T) {
			ctx := privacy.With(context.Background())
			s := privacy.From(ctx)
			recipient := s.Protect("jane@example.com")
			requests, calls := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests++
				for _, private := range []string{"jane@example.com", "history@example.com", "context@example.com", "Jane Doe", "my-password", "opaque-refresh", "ya29.privateToken", "signature-address", "old-quoted-secret"} {
					if strings.Contains(string(body), private) {
						t.Errorf("request %d leaked %q: %s", requests, private, body)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				var reply any
				text := s.Protect("Jane Doe wrote: meet tomorrow.")
				args := map[string]any{"query": recipient}
				if requests == 1 {
					switch provider {
					case "anthropic":
						reply = map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "call1", "name": "mail_GmailSearch", "input": args}}, "stop_reason": "tool_use"}
					case "gemini":
						reply = map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"functionCall": map[string]any{"name": "mail_GmailSearch", "args": args}}}}}}}
					default:
						encoded, _ := json.Marshal(args)
						reply = map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call1", "type": "function", "function": map[string]any{"name": "mail_GmailSearch", "arguments": string(encoded)}}}}}}}
					}
				} else {
					if !strings.Contains(string(body), "meet tomorrow") {
						t.Error("lost useful mail content")
					}
					switch provider {
					case "anthropic":
						reply = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "stop_reason": "end_turn"}
					case "gemini":
						reply = map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": text}}}}}}
					default:
						reply = map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": text}}}}
					}
				}
				_ = json.NewEncoder(w).Encode(reply)
			}))
			defer server.Close()
			m := gmai.New(provider, gmai.WithBaseURL(server.URL), gmai.WithAPIKey("provider-credential"), gmai.WithModel("test-model"), gmai.WithToolHandler(func(ctx context.Context, c gmai.ToolCall) gmai.ToolResult {
				calls++
				if c.Input["query"] != "jane@example.com" {
					t.Errorf("tool received %v", c.Input)
				}
				return gmai.ToolResult{ID: c.ID, Content: `{"from":"Jane Doe <jane@example.com>","text":"meet tomorrow\n> old-quoted-secret\n-- \nsignature-address","access_token":"ya29.privateToken","refresh_token":"opaque-refresh"}`}
			}))
			r, err := m.Generate(ctx, &gmai.Request{
				SystemPrompt: "Account: context@example.com",
				Prompt:       "Read mail from jane@example.com. password=my-password",
				Messages:     []gmai.Message{{Role: "user", Content: "Previous history@example.com"}},
				Tools:        []gmai.Tool{{Name: "mail_GmailSearch", Properties: map[string]any{"query": map[string]any{"type": "string"}}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || requests != 2 {
				t.Fatalf("tools=%d requests=%d", calls, requests)
			}
			if r.Answer != "Jane Doe wrote: meet tomorrow." && r.Reply != "Jane Doe wrote: meet tomorrow." {
				t.Fatalf("answer not restored: %+v", r)
			}
		})
	}
}
