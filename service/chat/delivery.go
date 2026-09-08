package chat

import (
	"context"
	"fmt"
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
	next := append(append([]RoomMessage(nil), room.Messages...), message)
	if len(next) > 20 {
		next = next[len(next)-20:]
	}
	if err := saveRoomMessages(room.ID, next); err != nil {
		return fmt.Errorf("chat message could not be saved: %w", err)
	}
	room.Messages = next
	room.LastActivity = time.Now()
	return nil
}
