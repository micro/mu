package chat

import (
	"context"
	"encoding/json"
	"mu/internal/event"
	"mu/internal/persist"
	"mu/internal/service"
	"testing"
)

func TestRoomArrivalAndArchiveCommitTogetherWithoutEventBodies(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	memberMu.Lock()
	members["dm_arrival"] = []string{"arrival-chat", "other-chat"}
	memberMu.Unlock()
	room := &Room{ID: "dm_arrival"}
	if err := room.keepMessage(RoomMessage{UserID: "arrival-chat", Content: "private room text", transport: "web"}); err != nil {
		t.Fatal(err)
	}
	names, err := persist.List("outbox/log")
	if err != nil {
		t.Fatal(err)
	}
	posted := 0
	for _, name := range names {
		b, err := persist.Read("outbox/log/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var e event.Record
		if err := json.Unmarshal(b, &e); err != nil {
			t.Fatal(err)
		}
		if e.Type != "chat.posted" {
			continue
		}
		posted++
		if len(e.Data) != 0 {
			t.Fatal("event should carry a source reference")
		}
		var rsp service.SourceResponse
		if err := (Server{}).Source(service.WithAccount(context.Background(), e.Account), &service.SourceRequest{ID: e.Resource}, &rsp); err != nil {
			t.Fatal(err)
		}
		if rsp.Item == nil || rsp.Item.Text != "private room text" || rsp.Item.Facts["transport"] != "web" || rsp.Item.Direction != "out" {
			t.Fatalf("%+v", rsp.Item)
		}
	}
	if posted != 1 || len(loadRoomMessages(room.ID)) != 1 {
		t.Fatal("room and event not committed")
	}
	if err := room.keepMessage(RoomMessage{UserID: "arrival-chat", Content: "tool post"}); err != nil {
		t.Fatal(err)
	}
	last := Everything("arrival-chat", 1)
	if len(last) != 1 || last[0].Facts["transport"] != "" {
		t.Fatal("tool post became interactive")
	}
}
