package inbox

import (
	"mu/internal/thread"
	"strings"
)

const (
	kindMessage = "message"
	kindNote    = "note"
	kindTask    = "task"
)

func kindOf(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case mailClient, thread.ChatClient, thread.SMSClient, thread.WhatsAppClient:
		return strings.ToLower(strings.TrimSpace(v))
	case kindMessage:
		return kindMessage
	case kindNote:
		return kindNote
	case kindTask:
		return kindTask
	}
	return ""
}
