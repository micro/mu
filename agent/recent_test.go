package agent_test

import (
	"encoding/json"
	"mu/agent"
	"mu/agent/work"
	"mu/home"
	"mu/inbox"
	"mu/internal/auth"
	"mu/internal/group"
	"mu/internal/thread"
	"mu/service/chat"
	"mu/service/tasks"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActivityFlowOwnershipAndGrouping(t *testing.T) {
	owner, member := "activity-owner", "activity-member"
	for _, id := range []string{owner, member} {
		auth.SetAccountForTest(&auth.Account{ID: id, Approved: true})
		defer auth.RemoveAccountForTest(id)
	}
	session, _ := auth.CreateSession(owner)
	memberSession, _ := auth.CreateSession(member)
	th := thread.Open(owner, thread.WebClient, "activity-test")
	thread.Name(owner, th.ID, "Plan the launch")
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Text: "Prepare the launch checklist", Role: thread.RolePerson})
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Text: "I’ll prepare the checklist and return the result here.", Role: thread.RoleAgent})
	task, err := tasks.CreateOn(owner, th.ID, "", "Prepare launch checklist", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tasks.Update(owner, task.ID, "", "", tasks.StatusDoing, "", ""); err != nil {
		t.Fatal(err)
	}
	foreign := thread.Open("activity-foreign", thread.WebClient, "secret")
	thread.Name(foreign.Account, foreign.ID, "Foreign secret")
	tasks.CreateOn(foreign.Account, foreign.ID, "", "Foreign work", "", tasks.Agent, time.Time{})
	rows := agent.RecentActivity(owner, true)
	if len(rows) != 1 || rows[0].ID != th.ID || rows[0].Status != tasks.StatusDoing {
		t.Fatalf("work not folded into its chat: %+v", rows)
	}
	request := func(handler http.HandlerFunc, path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: token})
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	statusPath := "/agent?view=activity-status&thread=" + th.ID
	status := request(agent.Handler, statusPath, session.Token)
	if status.Code != 200 || !strings.Contains(status.Body.String(), "Running") || strings.Contains(status.Body.String(), "Foreign") {
		t.Fatal(status.Code, status.Body.String())
	}
	if w := request(agent.Handler, statusPath, memberSession.Token); w.Code != 404 {
		t.Fatal("foreign work readable", w.Code)
	}
	if w := request(agent.Handler, statusPath, ""); w.Code != 401 {
		t.Fatal("anonymous work readable", w.Code)
	}
	g, err := group.Create(owner, "Family plans", false)
	if err != nil {
		t.Fatal(err)
	}
	room := group.General(g).ID
	group.Change(g.ID, owner, "invite", member, "")
	sharedThread := thread.Open(group.Scope(g.ID), thread.ChatClient, room)
	sharedTask, err := tasks.CreateOn(group.Scope(g.ID), sharedThread.ID, "", "Arrange shared visit", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	sharedPath := "/agent?view=activity-status&room=" + room
	if w := request(agent.Handler, sharedPath, memberSession.Token); w.Code != 404 {
		t.Fatal("invite allowed history", w.Code)
	}
	if err := group.Change(g.ID, member, "accept", "", ""); err != nil {
		t.Fatal(err)
	}
	if w := request(agent.Handler, sharedPath, memberSession.Token); w.Code != 200 || !strings.Contains(w.Body.String(), sharedTask.Title) || strings.Contains(w.Body.String(), task.Title) {
		t.Fatal("shared projection wrong", w.Code, w.Body.String())
	}
	found := false
	if dir := os.Getenv("ACTIVITY_PREVIEW_DIR"); dir != "" {
		empty := request(agent.ConsoleHandler, "/agent?view=chats", memberSession.Token)
		os.WriteFile(filepath.Join(dir, "empty.html"), empty.Body.Bytes(), 0600)
	}
	for _, c := range agent.RecentActivity(member, true) {
		if c.ID == room {
			found = c.Status == tasks.StatusTodo && strings.Contains(c.Context, g.Name)
		}
	}
	if !found {
		t.Fatal("shared task missing from group chat activity")
	}
	// Captures use the real handlers and shared shell for browser checks.
	capture := func(name string, handler http.HandlerFunc, path string) {
		if dir := os.Getenv("ACTIVITY_PREVIEW_DIR"); dir != "" {
			os.MkdirAll(dir, 0700)
			w := request(handler, path, session.Token)
			if w.Code != 200 {
				t.Fatalf("preview %s: %d %s", name, w.Code, w.Body.String())
			}
			if err := os.WriteFile(filepath.Join(dir, name+".html"), w.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	capture("home", home.Handler, "/home")
	capture("agents", agent.RosterHandler, "/agents")
	capture("scheduled", agent.RosterHandler, "/agents?view=scheduled")
	capture("feed", home.Handler, "/home?view=feed")
	capture("chats", agent.ConsoleHandler, "/agent?view=chats")
	capture("agent-chats", agent.ConsoleHandler, "/agent?view=chats&agent=micro")
	capture("new-chat", agent.ConsoleHandler, "/agent?view=new-chat")
	capture("activity", home.Handler, "/home?view=activity")
	capture("new", agent.ConsoleHandler, "/agent/micro?new=1")
	capture("chat", agent.ConsoleHandler, "/agent/micro?session="+th.ID)
	capture("group", chat.Handler, "/chat?id="+room)
	capture("work", work.Handler, "/work?id="+task.ID)
	mail := thread.Open(owner, "mail", "activity-mail")
	thread.Name(owner, mail.ID, "Launch notes")
	thread.Add(thread.Message{Account: owner, Thread: mail.ID, From: "colleague@example.test", Text: "Notes for the launch.", Role: thread.RolePerson})
	capture("inbox", inbox.Handler, "/inbox?id="+mail.ID)
	if dir := os.Getenv("ACTIVITY_PREVIEW_DIR"); dir != "" {
		var v map[string]any
		json.Unmarshal(status.Body.Bytes(), &v)
		b, _ := json.Marshal(v)
		os.WriteFile(filepath.Join(dir, "status.json"), b, 0600)
	}
	if err := group.Change(g.ID, owner, "remove", member, ""); err != nil {
		t.Fatal(err)
	}
	if w := request(agent.Handler, sharedPath, memberSession.Token); w.Code != 404 {
		t.Fatal("removed member retained activity", w.Code)
	}
	for _, c := range agent.RecentActivity(member, true) {
		if c.ID == room {
			t.Fatal("removed member retained activity row")
		}
	}
	if _, err = tasks.Update(owner, task.ID, "", "", tasks.StatusDone, "", "Checklist ready"); err != nil {
		t.Fatal(err)
	}
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "Checklist ready", Ref: "task-result:activity-test"})
	capture("chat-done", agent.ConsoleHandler, "/agent/micro?session="+th.ID)
	if dir := os.Getenv("ACTIVITY_PREVIEW_DIR"); dir != "" {
		done := request(agent.Handler, statusPath, session.Token)
		os.WriteFile(filepath.Join(dir, "status-done.json"), done.Body.Bytes(), 0600)
	}
	page := request(agent.ConsoleHandler, "/agent/micro?session="+th.ID, session.Token)
	if !strings.Contains(page.Body.String(), `data-delivery-id="task-result:activity-test"`) {
		t.Fatal("task output cannot be appended live")
	}
	otherChat := thread.Open(owner, thread.WebClient, "different-agent")
	thread.SetAgent(owner, otherChat.ID, "hello")
	thread.Name(owner, otherChat.ID, "Hello only chat")
	filtered := request(agent.ConsoleHandler, "/agent?view=chats&agent=micro", session.Token)
	if filtered.Code != 200 || !strings.Contains(filtered.Body.String(), "Plan the launch") || strings.Contains(filtered.Body.String(), "Hello only chat") || strings.Contains(filtered.Body.String(), "Arrange shared visit") {
		t.Fatal("agent chat scope mixed", filtered.Code)
	}
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RolePerson, Text: "Follow up after the checklist", At: time.Now().Add(time.Minute)})
	for _, c := range agent.RecentActivity(owner, true) {
		if c.ID == th.ID && c.Preview != "Follow up after the checklist" {
			t.Fatal("old outcome replaced newer message", c.Preview)
		}
	}
	// Terminal status alone does not mean the final message has arrived.
	awaiting, err := tasks.RecordOutcome(owner, task.ID, tasks.StatusDone, "Final outcome", "Final outcome", "Micro", nil)
	if err != nil {
		t.Fatal(err)
	}
	var poll struct {
		Pending    bool     `json:"pending"`
		Deliveries []string `json:"deliveries"`
	}
	w := request(agent.Handler, statusPath, session.Token)
	if err := json.Unmarshal(w.Body.Bytes(), &poll); err != nil || !poll.Pending {
		t.Fatal("undelivered outcome stopped polling", err)
	}
	ref := "task-result:" + awaiting.Delivery.ID
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "Final outcome", Ref: ref})
	if err := tasks.AcknowledgeDelivery(owner, task.ID, awaiting.Delivery.ID); err != nil {
		t.Fatal(err)
	}
	w = request(agent.Handler, statusPath, session.Token)
	if err := json.Unmarshal(w.Body.Bytes(), &poll); err != nil || poll.Pending {
		t.Fatal("terminal delivery kept polling", err)
	}
	found = false
	for _, id := range poll.Deliveries {
		if id == ref {
			found = true
		}
	}
	if !found {
		t.Fatal("client cannot detect missing terminal output")
	}

}
