package widgets

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"mu/app"
	"mu/data"
	"mu/reminder"
)

var (
	reminderMutex sync.RWMutex
	reminderHTML  string
)

// LoadReminder initializes the reminder data
func LoadReminder() {
	// Load cached HTML
	b, err := data.LoadFile("reminder.html")
	if err == nil {
		reminderMutex.Lock()
		reminderHTML = string(b)
		reminderMutex.Unlock()
	}

	// Start background refresh
	go refreshReminder()
}

func refreshReminder() {
	for {
		fetchReminder()
		time.Sleep(time.Hour)
	}
}

func fetchReminder() {
	app.Log("reminder", "Fetching reminder")

	resp, err := http.Get("https://reminder.dev/api/latest")
	if err != nil {
		app.Log("reminder", "Error fetching: %v", err)
		return
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)

	var val map[string]interface{}
	if err := json.Unmarshal(b, &val); err != nil {
		app.Log("reminder", "Error parsing: %v", err)
		return
	}

	// Save full JSON data for the reminder page
	data.SaveFile("reminder.json", string(b))
	
	// Load the reminder data to check if we need to generate context
	var reminderData reminder.ReminderData
	if err := json.Unmarshal(b, &reminderData); err == nil {
		// Request context generation if needed
		reminder.RequestReminderContext(&reminderData)
	}

	link := "/reminder"

	html := fmt.Sprintf(`<div class="item"><div class="verse">%s</div></div>`, val["verse"])
	html += app.Link("More", link)

	reminderMutex.Lock()
	reminderHTML = html
	data.SaveFile("reminder.html", html)
	reminderMutex.Unlock()

	// Index for search/RAG
	verse := val["verse"].(string)
	name := ""
	if v, ok := val["name"]; ok {
		name = v.(string)
	}
	hadith := ""
	if h, ok := val["hadith"]; ok {
		hadith = h.(string)
	}
	message := ""
	if m, ok := val["message"]; ok {
		message = m.(string)
	}
	updated := ""
	if u, ok := val["updated"]; ok {
		updated = u.(string)
	}

	content := fmt.Sprintf("Name of Allah: %s\n\nVerse: %s\n\nHadith: %s\n\n%s", name, verse, hadith, message)

	// Index with ID "daily" (not "reminder_daily") because the chat room type extraction
	// will split "reminder_daily" into type="reminder" and id="daily", then look up just "daily"
	data.Index(
		"daily",
		"reminder",
		"Daily Islamic Reminder",
		content,
		map[string]interface{}{
			"url":     "/reminder",
			"updated": updated,
			"source":  "daily",
		},
	)

	app.Log("reminder", "Updated reminder")
}

// ReminderHTML returns the rendered reminder card HTML
func ReminderHTML() string {
	reminderMutex.RLock()
	defer reminderMutex.RUnlock()
	return reminderHTML
}
