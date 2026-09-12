package prayer

import (
	"strings"
	"testing"

	"mu/internal/service"
)

func TestReminderCardShowsVerseAndReflection(t *testing.T) {
	body := renderReminderCard(&ReminderData{Verse: "Verse & reference", Message: "Reflection <script>"})
	if !strings.Contains(body, "Verse &amp; reference") || !strings.Contains(body, "Reflection &lt;script&gt;") {
		t.Fatal("card must retain and escape both verse and reflection")
	}
	reminderMutex.Lock()
	previous := reminderHTML
	reminderHTML = body
	reminderMutex.Unlock()
	defer func() { reminderMutex.Lock(); reminderHTML = previous; reminderMutex.Unlock() }()
	got := ReminderHTML(service.Anyone())
	if got != body {
		t.Fatal("card must not append prayer times or location scripts")
	}
}
