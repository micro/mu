package agent

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/thread"
	pb "mu/proto/assistant/v1"
)

func mucpCall(token, endpoint, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/mucp", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Micro-Protocol", "mucp/1")
	r.Header.Set("Micro-Service", "assistant")
	r.Header.Set("Micro-Endpoint", "Assistant."+endpoint)
	r.Header.Set("Micro-Id", "test-request-123456789")
	w := httptest.NewRecorder()
	MUCPHandler(w, r)
	return w
}

func TestMUCPBoundary(t *testing.T) {
	const owner, other = "mucp_owner", "mucp_other"
	for _, id := range []string{owner, other} {
		if err := auth.Create(&auth.Account{ID: id, Admin: true}); err != nil {
			t.Fatal(err)
		}
	}
	_, read, err := auth.CreateToken(owner, "read", []string{"read", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, write, err := auth.CreateToken(owner, "write", []string{"read", "write", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, service, err := auth.CreateToken(owner, "service", []string{"read", "write", auth.ScopePrefix + "news"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	foreign := thread.Open(other, thread.WebClient, "foreign")
	mail := thread.Open(owner, "mail", "mail")
	for _, tc := range []struct {
		token, endpoint, body string
		status                int
	}{
		{read, "ListAgents", "{}", 200},
		{service, "ListAgents", "{}", 403},
		{"invalid", "ListAgents", "{}", 401},
		{read, "Ask", `{"prompt":"hello","mode":"ACCESS_MODE_PERSONAL"}`, 403},
		{write, "Ask", `{"prompt":"hello"}`, 400},
		{write, "Ask", `{"prompt":"hello","mode":77}`, 400},
		{write, "Ask", `{"prompt":"hello","mode":"ACCESS_MODE_PUBLIC","thread":"private"}`, 400},
		{read, "ListAgents", `{"account":"mucp_other"}`, 400},
		{read, "ListThreads", `{"pageSize":1,"pageSize":2}`, 400},
		{read, "ReadThread", `{"thread":"` + foreign.ID + `"}`, 404},
		{read, "ReadThread", `{"thread":"` + mail.ID + `"}`, 404},
		{read, "ListThreads", `{"pageSize":101}`, 400},
	} {
		w := mucpCall(tc.token, tc.endpoint, tc.body)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.endpoint, tc.body, w.Code, w.Body.String())
		}
		if w.Header().Get("Micro-Id") != "test-request-123456789" {
			t.Fatal("missing correlation")
		}
	}
}

func TestMUCPPaginationIsolation(t *testing.T) {
	const owner = "mucp_pages"
	if err := auth.Create(&auth.Account{ID: owner, Admin: true}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a", "b", "c"} {
		thread.Open(owner, thread.WebClient, key)
	}
	first, err := assistantThreads(owner, &pb.ListThreadsRequest{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Threads) != 1 || first.NextPageToken == "" {
		t.Fatal("missing page")
	}
	second, err := assistantThreads(owner, &pb.ListThreadsRequest{PageSize: 1, PageToken: first.NextPageToken})
	if err != nil {
		t.Fatal(err)
	}
	if first.Threads[0].Id == second.Threads[0].Id {
		t.Fatal("repeated thread")
	}
	for _, q := range []*pb.ListThreadsRequest{
		{PageToken: first.NextPageToken + "x"},
		{AgentId: "micro", PageToken: first.NextPageToken},
	} {
		if _, err := assistantThreads(owner, q); err == nil {
			t.Fatal("unbound page token")
		}
	}
	if _, err := assistantThreads("other", &pb.ListThreadsRequest{PageToken: first.NextPageToken}); err == nil {
		t.Fatal("cross-account token")
	}
	th := first.Threads[0].Id
	for _, text := range []string{"one", "two", "three"} {
		thread.Add(thread.Message{Account: owner, Thread: th, Role: thread.RolePerson, Text: text})
	}
	page, err := assistantThread(owner, &pb.ReadThreadRequest{Thread: th, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || page.NextPageToken == "" {
		t.Fatal("missing message page")
	}
	end, err := assistantThread(owner, &pb.ReadThreadRequest{Thread: th, PageSize: 2, PageToken: page.NextPageToken})
	if err != nil {
		t.Fatal(err)
	}
	if len(end.Messages) != 1 || end.NextPageToken != "" {
		t.Fatal("incorrect last page")
	}
}

func TestMUCPDisconnectKeepsAcceptedWork(t *testing.T) {
	replies.once.Do(func() {})
	replies.jobs = map[string]pendingReply{}
	replies.active = map[string]string{}
	const owner = "mucp_disconnect"
	if err := auth.Create(&auth.Account{ID: owner, Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_, token, err := auth.CreateToken(owner, "client", []string{"read", "write", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("POST", "/mucp", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	_, err = assistantAsk(w, r, owner, "disconnect-123456789", &pb.AskRequest{Prompt: "hello", Mode: pb.AccessMode_ACCESS_MODE_PERSONAL})
	if err == nil {
		t.Fatal("expected disconnected wait")
	}
	id := w.Header().Get("Micro-Thread")
	if id == "" || len(replies.jobs) != 1 {
		t.Fatal("request was not durably accepted")
	}
	calls := 0
	processReply(func(q AskRequest) (Answer, error) {
		calls++
		recordAnswer(owner, q.On, "Recovered answer", "", "", q.AnswerRef)
		return Answer{Text: "Recovered answer"}, nil
	})
	processReply(func(AskRequest) (Answer, error) { t.Fatal("executed twice"); return Answer{}, nil })
	page, err := assistantThread(owner, &pb.ReadThreadRequest{Thread: id})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	if calls != 1 || page.Pending || !strings.Contains(string(raw), "Recovered answer") {
		t.Fatalf("missing recovered outcome: %s", raw)
	}
}

func TestMUCPSharedWireFixture(t *testing.T) {
	raw, err := os.ReadFile("../proto/testdata/ask-response.json")
	if err != nil {
		t.Fatal(err)
	}
	var response pb.AskResponse
	if err = (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if response.Thread != "thread-example" || len(response.Content) != 2 || response.Content[1].Fallback != "A future widget can still be read" {
		t.Fatal("wire fixture lost fallback")
	}
}

func TestMUCPBrowserSessionRequiresCSRFForWrites(t *testing.T) {
	const id = "mucp_browser_session"
	if err := auth.Create(&auth.Account{ID: id, Admin: true}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, csrf := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"prompt":"hello"}`))
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		r.Header.Set("Micro-Protocol", "mucp/1")
		r.Header.Set("Micro-Service", "assistant")
		r.Header.Set("Micro-Endpoint", "Assistant.Ask")
		r.Header.Set("Micro-Id", "browser-request-1234")
		r.Header.Set("Content-Type", "application/json")
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		MUCPHandler(w, r)
		want := 403
		if csrf {
			want = 400
		} // Missing access mode reaches validation only after authorization.
		if w.Code != want {
			t.Fatalf("csrf=%v: %d %s", csrf, w.Code, w.Body.String())
		}
	}
}

func TestMUCPSharedExchanges(t *testing.T) {
	raw, err := os.ReadFile("../proto/testdata/exchanges.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Operation         string
		Request, Response json.RawMessage
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, item := range cases {
		var request, response proto.Message
		switch item.Operation {
		case "ListAgents":
			request = &pb.ListAgentsRequest{}
			response = &pb.ListAgentsResponse{}
		case "ListThreads":
			request = &pb.ListThreadsRequest{}
			response = &pb.ListThreadsResponse{}
		case "ReadThread":
			request = &pb.ReadThreadRequest{}
			response = &pb.ReadThreadResponse{}
		case "Ask":
			request = &pb.AskRequest{}
			response = &pb.AskResponse{}
		default:
			t.Fatal("unknown operation", item.Operation)
		}
		if err := protojson.Unmarshal(item.Request, request); err != nil {
			t.Fatal(item.Operation, err)
		}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(item.Response, response); err != nil {
			t.Fatal(item.Operation, err)
		}
	}
}
