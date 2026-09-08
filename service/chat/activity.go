package chat

import (
	"fmt"
	"net/url"
	"strings"

	"mu/internal/event"
)

func announceMessage(roomID string, message RoomMessage) {
	if message.System || strings.TrimSpace(message.Content) == "" {
		return
	}
	link := "/chat?id=" + url.QueryEscape(roomID) + fmt.Sprintf("#message-%d", message.Timestamp.UnixNano())
	text := message.UserID + ": " + message.Content
	if Private(roomID) {
		for _, account := range Members(roomID) {
			event.Announce("chat", text, link, account)
		}
		return
	}
	if Listable(roomID) {
		event.Announce("chat", text, link, "")
	}
}
