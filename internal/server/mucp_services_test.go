package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/group"
	"mu/internal/service"
	chatpb "mu/proto/service/chat/v1"
	grouppb "mu/proto/service/groups/v1"
	"mu/service/chat"
	"mu/service/groups"
)

func TestImplementedServiceContracts(t *testing.T) {
	groups.Load()
	chat.LoadService()
	const owner = "contract_group_owner"
	const other = "contract_group_other"
	for _, id := range []string{owner, other} {
		auth.SetAccountForTest(&auth.Account{ID: id, Admin: true})
		defer auth.RemoveAccountForTest(id)
	}
	token := func(id string, scopes ...string) string {
		_, key, err := auth.CreateToken(id, "contracts", scopes, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	own := token(owner, "read", "write", "service:groups", "service:chat")
	foreign := token(other, "read", "write", "service:groups", "service:chat")
	product := token(owner, "read", "write", "api:agent")
	readonly := token(owner, "read", "service:groups")
	w := compositionCall(own, "groups", "Groups.Create", `{"name":"Contract family"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct {
		Group struct {
			ID      string `json:"id"`
			Created string `json:"created"`
		} `json:"group"`
		Chat string `json:"chat"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created.Group.ID
	if id == "" {
		t.Fatal(w.Body.String())
	}
	defer group.Change(id, owner, "delete", "", "")
	if _, err := time.Parse(time.RFC3339Nano, created.Group.Created); err != nil {
		t.Fatal("timestamp is not protobuf JSON", err)
	}
	for _, tc := range []struct {
		token, svc, method, body string
		code                     int
	}{
		{product, "groups", "Groups.List", "{}", 403},
		{readonly, "groups", "Groups.Create", `{"name":"Forbidden"}`, 403},
		{own, "groups", "Groups.Read", `{"id":"` + id + `"}`, 200},
		{own, "groups", "Groups.Create", `{"name":""}`, 400},
		{own, "groups", "Groups.Update", `{"id":"` + id + `","action":"unknown"}`, 400},
		{own, "chat", "Chat.Send", `{"room":"` + group.RoomPrefix + id + `","message":"` + strings.Repeat("a", 8001) + `"}`, 400},
		{own, "chat", "Chat.Send", `{"room":"` + group.RoomPrefix + id + `","message":"Hello together"}`, 200},
		{own, "chat", "Chat.Messages", `{"room":"` + group.RoomPrefix + id + `"}`, 200},
		{own, "groups", "Groups.Read", `{"id":"` + id + `","account":"` + other + `"}`, 400},
	} {
		w = compositionCall(tc.token, tc.svc, tc.method, tc.body)
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.method, w.Code, w.Body.String())
		}
	}
	for _, method := range []string{"Groups.Read", "Chat.Messages"} {
		svc, body := "groups", `{"id":"`+id+`"}`
		if method == "Chat.Messages" {
			svc = "chat"
			body = `{"room":"` + group.RoomPrefix + id + `"}`
		}
		w = compositionCall(foreign, svc, method, body)
		if w.Code != 404 || strings.Contains(w.Body.String(), "Hello together") || strings.Contains(w.Body.String(), "Contract family") {
			t.Fatal("foreign account read group", w.Code, w.Body.String())
		}
	}
	// Generated types travel through go-micro's gateway, not just the direct handler.
	var messages chatpb.MessagesResponse
	if err := service.Call(service.WithAccount(context.Background(), owner), "chat", "Server.Messages", &chatpb.MessagesRequest{Room: group.RoomPrefix + id}, &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages.Messages) != 1 || messages.Messages[0].Timestamp == nil || messages.Messages[0].Content != "Hello together" {
		t.Fatal(&messages)
	}
	var read grouppb.GroupResponse
	if err := service.Call(service.WithAccount(context.Background(), owner), "groups", "Server.Read", &grouppb.ReadRequest{Id: id}, &read); err != nil {
		t.Fatal(err)
	}
	if read.Group.GetId() != id {
		t.Fatal(&read)
	}
	// Legacy JSON callers still receive RFC3339 timestamps and integer counts.
	var legacy map[string]any
	if err := service.Call(service.WithAccount(context.Background(), owner), "groups", "Server.Read", map[string]any{"id": id}, &legacy); err != nil {
		t.Fatal(err)
	}
	record := legacy["group"].(map[string]any)
	if _, ok := record["spend_limit"].(float64); !ok {
		t.Fatal("legacy count changed", record)
	}
	for _, key := range []string{"links", "link_hash"} {
		if _, ok := record[key]; ok {
			t.Fatal("storage credential in contract", key)
		}
	}
}
