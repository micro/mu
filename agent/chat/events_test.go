package chat

import (
	"mu/internal/event"
	"mu/internal/service"
	svc "mu/service/chat"
	"testing"
)

func TestStoredChatFactsCrossAPIWithoutMakingToolPostsInstructions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := service.Register(svc.Spec); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		transport, account string
		want               bool
	}{{"web", "chat-event-owner", true}, {"", "chat-event-owner", false}, {"web", "another-account", false}} {
		id, err := svc.KeepSaved("chat-event-owner", svc.Said{Conv: "news_event-test", From: "chat-event-owner", Text: "@micro help", Facts: map[string]interface{}{"transport": tc.transport, "participants": 1}})
		if err != nil {
			t.Fatal(err)
		}
		action, err := prepareChat(event.Record{Type: "chat.posted", Service: "chat", Account: tc.account, Resource: id})
		if err != nil {
			t.Fatal(err)
		}
		if (action != nil) != tc.want {
			t.Fatalf("transport=%q account=%q action=%v", tc.transport, tc.account, action != nil)
		}
	}
}
