package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"mu/internal/group"
	"strings"
	"time"
)

// post waits for the room to persist the message, not for a browser to remain
// connected. A stopped room or a failed disk write must not report "sent".
func post(ctx context.Context, room *Room, message RoomMessage) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	message.ack = make(chan error, 1)
	select {
	case room.Broadcast <- message:
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("chat room is busy or stopped")
	}
	select {
	case err := <-message.ack:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (room *Room) keepMessage(message RoomMessage) error {
	room.mutex.Lock()
	defer room.mutex.Unlock()
	if !message.System && !message.IsLLM && !Member(room.ID, message.UserID) {
		return fmt.Errorf("no access to this chat room")
	}
	if isGroup(room.ID) && !message.System {
		g, ok := group.Details(groupID(room.ID))
		if !ok || !Member(room.ID, message.UserID) {
			return fmt.Errorf("no access to this group")
		}
		if g.Encrypted != (message.OMEMO != "") {
			return fmt.Errorf("message encryption does not match the group; use its selected chat mode")
		}
	}
	next := append(append([]RoomMessage(nil), room.Messages...), message)
	if !isGroup(room.ID) && len(next) > 20 {
		next = next[len(next)-20:]
	}
	var err error
	if isGroup(room.ID) {
		err = saveRoomMessages(room.ID, next)
	} else if !message.System && !message.IsLLM {
		var b []byte
		b, err = json.Marshal(next)
		if err == nil {
			_, err = keepSaved(message.UserID, Said{Conv: room.ID, From: message.UserID, Text: message.Content, At: message.Timestamp,
				Facts: map[string]interface{}{"transport": message.transport, "room": room.ID, "title": room.Title, "summary": room.Summary, "url": room.URL, "participants": len(room.Clients), "direct": microDM(room.ID, message.UserID)}},
				map[string][]byte{"room_" + strings.ReplaceAll(room.ID, "/", "_") + ".json": b}, "chat.posted")
		}
	} else {
		err = saveRoomMessages(room.ID, next)
	}
	if err != nil {
		return fmt.Errorf("chat message could not be saved: %w", err)
	}
	room.Messages = next
	room.LastActivity = time.Now()
	return nil
}
