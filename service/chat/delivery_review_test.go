package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/event"
	"mu/internal/service"
)

func TestDeletedPrivateMemberCannotReopenOldTranscript(t *testing.T) {
	const a, b = "deletedparticipant", "remainingparticipant"
	id := PairRoom(a, b)
	Open(id, a, b)
	room := deliveryRoom(t, id)
	if err := post(context.Background(), room, RoomMessage{UserID: a, Content: "Old private conversation"}); err != nil {
		t.Fatal(err)
	}
	Forget(a)
	loadPrivate()
	if Member(id, a) || !Member(id, b) {
		t.Fatal("deletion did not persist the correct membership")
	}
	var rsp MessagesResponse
	if err := (Server{}).Messages(service.WithAccount(context.Background(), a), &MessagesRequest{Room: id}, &rsp); err == nil {
		t.Fatal("deleted name can read old transcript")
	}
	if err := (Server{}).Messages(service.WithAccount(context.Background(), b), &MessagesRequest{Room: id}, &rsp); err != nil || len(rsp.Messages) != 1 {
		t.Fatalf("surviving participant lost conversation: %v", err)
	}
	fresh := PairRoom(a, b)
	if fresh == "" || fresh == id || Member(fresh, a) {
		t.Fatal("reused name inherited old room or premature access")
	}
	Open(fresh, a, b)
	loadPrivate()
	if Member(id, a) || PairRoom(b, a) != fresh || len(loadRoomMessages(fresh)) != 0 {
		t.Fatal("recreated account inherited old messages")
	}
	if err := post(context.Background(), room, RoomMessage{UserID: a, Content: "Revoked sender"}); err == nil {
		t.Fatal("queued message bypassed revocation")
	}
	Forget(b)
	if _, err := data.LoadFile("room_" + id + ".json"); !os.IsNotExist(err) {
		t.Fatalf("empty room transcript retained: %v", err)
	}
}

func TestLegacyPrivateMembershipRetainsRoom(t *testing.T) {
	memberMu.Lock()
	oldMembers, oldPairs := members, pairIDs
	memberMu.Unlock()
	t.Cleanup(func() { memberMu.Lock(); members, pairIDs = oldMembers, oldPairs; _ = savePrivate(); memberMu.Unlock() })
	if err := data.SaveJSON("chat_private.json", map[string][]string{"dm_legacy_room": {"legacy-a", "legacy-b"}}); err != nil {
		t.Fatal(err)
	}
	loadPrivate()
	if got := PairRoom("legacy-a", "legacy-b"); got != "dm_legacy_room" {
		t.Fatalf("lost legacy room: %s", got)
	}
	loadPrivate()
	if !Member("dm_legacy_room", "legacy-a") {
		t.Fatal("migration lost membership")
	}
}

func TestWebsocketSavesBeforeRequestingAgent(t *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "websocketsaved"
		if failed {
			name = "websocketfailed"
		}
		t.Run(name, func(t *testing.T) {
			if err := auth.Create(&auth.Account{ID: name, Name: name}); err != nil {
				t.Fatal(err)
			}
			sess, err := auth.CreateSession(name)
			if err != nil {
				t.Fatal(err)
			}
			room := deliveryRoom(t, "chat_"+name)
			if failed {
				path := filepath.Join(os.Getenv("HOME"), ".mu", "data", "room_"+room.ID+".json")
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.RemoveAll(path) })
			}
			sub := event.Subscribe(event.ChatForAgent)
			defer sub.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handleWebSocket(w, r, room) }))
			defer server.Close()
			header := http.Header{"Cookie": []string{"session=" + sess.Token}}
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), header)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			// The initial presence frame confirms registration is complete.
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatal(err)
			}
			if err := conn.WriteJSON(map[string]string{"content": "@micro explain durable correspondence"}); err != nil {
				t.Fatal(err)
			}
			if failed {
				_, _, err := conn.ReadMessage()
				if !websocket.IsCloseError(err, websocket.CloseTryAgainLater) {
					t.Fatalf("no visible persistence failure: %v", err)
				}
				select {
				case e := <-sub.Chan:
					t.Fatalf("unsaved prompt requested agent: %+v", e)
				case <-time.After(30 * time.Millisecond):
				}
			} else {
				select {
				case <-sub.Chan:
				case <-time.After(3 * time.Second):
					t.Fatal("saved prompt not dispatched")
				}
				stored := loadRoomMessages(room.ID)
				if len(stored) != 1 || stored[0].Content != "@micro explain durable correspondence" {
					t.Fatalf("dispatch before persistence: %+v", stored)
				}
			}
		})
	}
}
