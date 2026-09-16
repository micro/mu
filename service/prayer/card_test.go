package prayer

import (
	"strings"
	"testing"

	"mu/internal/service"
)

func TestReminderCardShowsOnlyVerse(t *testing.T) {
	body := renderReminderCard(&ReminderData{Verse: "Verse & reference", Message: "Reflection <script>"})
	if !strings.Contains(body, "Verse &amp; reference") {
		t.Fatal("card must retain and escape the verse")
	}
	if strings.Contains(body, "Reflection") {
		t.Fatal("card must not include the reflection")
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
