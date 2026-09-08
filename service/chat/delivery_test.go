package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"mu/internal/service"
)

func deliveryRoom(t *testing.T, id string) *Room {
	t.Helper()
	room := &Room{ID: id, Clients: make(map[*websocket.Conn]*Client), Register: make(chan *Client), Unregister: make(chan *Client), Broadcast: make(chan RoomMessage, 32), Shutdown: make(chan bool)}
	roomsMutex.Lock()
	old := rooms[id]
	rooms[id] = room
	roomsMutex.Unlock()
	done := make(chan struct{})
	go func() { defer close(done); room.run() }()
	t.Cleanup(func() {
		close(room.Shutdown)
		<-done
		roomsMutex.Lock()
		if old == nil {
			delete(rooms, id)
		} else {
			rooms[id] = old
		}
		roomsMutex.Unlock()
	})
	return room
}

func TestChatSendIsStoredBeforeAcknowledgementAndSurvivesDisconnect(t *testing.T) {
	for _, id := range []string{"chat_delivery", "news_delivery", "dm_delivery"} {
		t.Run(id, func(t *testing.T) {
			Open(id, "alice", "bob")
			room := deliveryRoom(t, id)
			var rsp SendResponse
			if err := (Server{}).Send(service.WithAccount(context.Background(), "alice"), &SendRequest{Room: id, Message: "The completed answer"}, &rsp); err != nil {
				t.Fatal(err)
			}
			// No browser was connected. A success must still have a durable copy.
			stored := loadRoomMessages(id)
			if rsp.Result != "sent" || len(stored) != 1 || stored[0].Content != "The completed answer" {
				t.Fatalf("acknowledged without saving: %+v", stored)
			}
			roomsMutex.Lock()
			delete(rooms, id)
			roomsMutex.Unlock()
			var history MessagesResponse
			if err := (Server{}).Messages(service.WithAccount(context.Background(), "bob"), &MessagesRequest{Room: id}, &history); err != nil || len(history.Messages) != 1 {
				t.Fatalf("reconnect lost answer: %+v, %v", history, err)
			}
			if Private(id) {
				if err := (Server{}).Messages(service.WithAccount(context.Background(), "stranger"), &MessagesRequest{Room: id}, &history); err == nil {
					t.Fatal("private transcript leaked")
				}
			}
			roomsMutex.Lock()
			rooms[id] = room
			roomsMutex.Unlock()
		})
	}
}

func TestChatStorageFailureIsNotAcknowledgedOrPublished(t *testing.T) {
	id := "chat_failed_save"
	room := deliveryRoom(t, id)
	path := filepath.Join(os.Getenv("HOME"), ".mu", "data", "room_"+id+".json")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(path) })
	var rsp SendResponse
	if err := (Server{}).Send(service.WithAccount(context.Background(), "alice"), &SendRequest{Room: id, Message: "Must be saved"}, &rsp); err == nil {
		t.Fatal("failed storage reported sent")
	}
	room.mutex.RLock()
	n := len(room.Messages)
	room.mutex.RUnlock()
	if n != 0 || rsp.Result == "sent" {
		t.Fatal("unpersisted message became visible")
	}
	if Say(id, AgentName, "Another answer") {
		t.Fatal("agent delivery hid a disk failure")
	}
}

func TestStoppedChatDeliveryIsBounded(t *testing.T) {
	room := &Room{ID: "chat_stopped", Broadcast: make(chan RoomMessage, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := post(ctx, room, RoomMessage{Content: "No reader"}); err == nil {
		t.Fatal("stopped room reported success")
	}
	// The first message fills the queue; the next caller must fail immediately.
	start := time.Now()
	if err := post(context.Background(), room, RoomMessage{Content: "Still stopped"}); err == nil {
		t.Fatal("full room reported success")
	}
	if time.Since(start) > time.Second {
		t.Fatal("full room blocked the caller")
	}
}

func TestConcurrentChatSavesKeepEveryAcknowledgedMessage(t *testing.T) {
	room := deliveryRoom(t, "chat_concurrent_delivery")
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := post(context.Background(), room, RoomMessage{UserID: "alice", Content: fmt.Sprintf("answer %d", i), Timestamp: time.Now()}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if got := loadRoomMessages(room.ID); len(got) != 12 {
		t.Fatalf("saved %d of 12 acknowledged answers", len(got))
	}
}

func TestOfflineXMPPAnswerIsAvailableWhenTheClientReconnects(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "offlinereply")
	if SayTo(acc.ID, AgentAddress(), "Your task is finished") {
		t.Fatal("offline account reported present")
	}
	// Reload the persisted archive before connecting a real test client.
	LoadStore()
	c := dial(t)
	defer c.Close()
	c.handshake(t, acc.ID, token)
	c.write(mamQuery("offline-answer", AgentAddress(), ""))
	got := c.until(t, "</iq>")
	if !strings.Contains(got, "Your task is finished") || !strings.Contains(got, "urn:xmpp:delay") {
		t.Fatalf("offline answer missing from archive: %s", clip(got))
	}
}

func TestChatArchiveWriteFailureRollsBackMemory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".mu", "data", "chat.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if id, err := KeepSaved("failed-archive", Said{Conv: "test", Text: "Unsaved"}); err == nil || id != "" {
		t.Fatal("unpersisted archive entry reported saved")
	}
	if len(Conversation("failed-archive", "test", 10)) != 0 {
		t.Fatal("failed save changed visible archive")
	}
}

func TestLiveXMPPAnswerUsesItsArchiveMessageID(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "livereply")
	c := dial(t)
	defer c.Close()
	c.handshake(t, acc.ID, token)
	if !SayTo(acc.ID, AgentAddress(), "Here is the answer") {
		t.Fatal("connected client was not delivered to")
	}
	got := c.until(t, "</message>")
	history := Conversation(acc.ID, xmppRoom(AgentAddress(), acc.ID+"@"+Domain()), 10)
	if len(history) != 1 || !strings.Contains(got, "id='"+history[0].ID+"'") {
		t.Fatalf("live and archived IDs differ: %s", clip(got))
	}
}

func TestQuietRoomDoesNotExpireAnUnreadAnswerOvernight(t *testing.T) {
	id := "news_quiet_delivery"
	if err := saveRoomMessages(id, []RoomMessage{{UserID: AgentName, Content: "The saved outcome", Timestamp: time.Now().Add(-48 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if got := loadRoomMessages(id); len(got) != 1 || got[0].Content != "The saved outcome" {
		t.Fatal("quiet room expired the answer before it was read")
	}
}
