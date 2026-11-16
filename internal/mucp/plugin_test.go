package mucp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"mu/internal/auth"
	pb "mu/proto/assistant/v1"
)

func TestAssistantPlugin(t *testing.T) {
	const owner = "plugin_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: "Plugin test", Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	token := func(permissions ...string) string {
		_, key, err := auth.CreateToken(owner, "plugin", permissions, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	full := token("read", "write", "api:agent", "api:inbox")
	read := token("read", "api:agent")
	service := token("read", "write", auth.ScopePrefix+"news")
	calls := 0
	handler := PluginHandler([]Operation{
		{Service: "assistant", Endpoint: "Assistant.Submit", Capability: "agent", Writes: true, Request: func() proto.Message { return &pb.SubmitRequest{} }, Handle: func(_ http.ResponseWriter, _ *http.Request, account, id string, q proto.Message) (proto.Message, error) {
			calls++
			in := q.(*pb.SubmitRequest)
			if account != owner || !ValidID(id) || in.MessageId != "stable_request_12345" || in.Request.Mode != pb.AccessMode_ACCESS_MODE_PERSONAL || in.Request.Prompt != "hello" || in.Request.AgentId != "chosen" {
				t.Fatalf("bad dispatch account=%s request=%+v", account, in)
			}
			return &pb.Operation{Id: "operation", State: pb.OperationState_OPERATION_STATE_QUEUED}, nil
		}},
		{Service: "assistant", Endpoint: "Assistant.ReadOperation", Capability: "agent", Request: func() proto.Message { return &pb.ReadOperationRequest{} }, Handle: func(_ http.ResponseWriter, _ *http.Request, account, id string, q proto.Message) (proto.Message, error) {
			return &pb.Operation{Id: q.(*pb.ReadOperationRequest).Id}, nil
		}},
	})
	call := func(key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://mu.example/mcp/assistant", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	listed := call("", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var envelope struct {
		Result struct {
			Tools []struct {
				Name        string          `json:"name"`
				Security    []any           `json:"securitySchemes"`
				Annotations map[string]bool `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err, listed.Body.String())
	}
	if len(envelope.Result.Tools) != 6 {
		t.Fatal(listed.Body.String())
	}
	for _, tool := range envelope.Result.Tools {
		if len(tool.Security) == 0 {
			t.Fatal("missing auth metadata")
		}
		if tool.Annotations["readOnlyHint"] == (tool.Name == "ask") {
			t.Fatalf("wrong annotations for %s", tool.Name)
		}
	}
	ask := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ask","arguments":{"requestId":"stable_request_12345","prompt":"hello","agentId":"chosen"}}}`
	unauth := call("", ask)
	if unauth.Code != 401 || !strings.Contains(unauth.Header().Get("WWW-Authenticate"), "oauth-protected-resource") {
		t.Fatal(unauth.Code, unauth.Body.String())
	}
	for _, key := range []string{read, service} {
		w := call(key, ask)
		if !strings.Contains(w.Body.String(), `"isError":true`) {
			t.Fatal(w.Body.String())
		}
	}
	if calls != 0 {
		t.Fatal("unauthorized dispatch")
	}
	w := call(full, ask)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"isError":true`) || !strings.Contains(w.Body.String(), "QUEUED") {
		t.Fatal(w.Code, w.Body.String())
	}
	if calls != 1 {
		t.Fatal("submission count", calls)
	}
	w = call(full, strings.Replace(ask, `"agentId":"chosen"`, `"account":"other"`, 1))
	if !strings.Contains(w.Body.String(), `"isError":true`) || calls != 1 {
		t.Fatal("identity argument accepted", w.Body.String())
	}
	w = call(full, strings.Replace(ask, `"name":"ask"`, `"name":"mail_send"`, 1))
	if !strings.Contains(w.Body.String(), `"error"`) || calls != 1 {
		t.Fatal("raw service reachable", w.Body.String())
	}
	w = call(read, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"status","arguments":{"id":"operation"}}}`)
	if strings.Contains(w.Body.String(), `"isError":true`) {
		t.Fatal(w.Body.String())
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/mcp/assistant", strings.NewReader(ask))
	req.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != 401 {
		t.Fatal("cookie authorized plugin", w.Code)
	}
}
