package agent

import (
	"context"
	"encoding/json"
	"net/http/httptest"
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
