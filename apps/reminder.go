package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"mu/app"
	"mu/data"
	"mu/tools"
)

var (
	reminderMutex sync.RWMutex
	reminderHTML  string
)

// LoadReminder initializes the reminder data
func LoadReminder() {
	// Register tools
	RegisterReminderTools()

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

	link := fmt.Sprintf("https://reminder.dev%s", val["links"].(map[string]interface{})["verse"].(string))

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

	data.Index(
		"reminder_card_daily",
		"reminder",
		"Daily Islamic Reminder",
		content,
		map[string]interface{}{
			"url":     link,
			"updated": updated,
			"source":  "card",
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

// RegisterReminderTools registers reminder tools with the tools registry
func RegisterReminderTools() {
	tools.Register(tools.Tool{
		Name:        "reminder.today",
		Description: "Get today's daily Islamic reminder (Quranic verse and hadith)",
		Category:    "reminder",
		Path:        "/api/reminder",
		Method:      "GET",
		Output: map[string]tools.Param{
			"verse":   {Type: "string", Description: "Quranic verse"},
			"hadith":  {Type: "string", Description: "Related hadith"},
			"name":    {Type: "string", Description: "Name of Allah"},
			"message": {Type: "string", Description: "Daily message"},
		},
		Handler: handleGetReminder,
	})
}

func handleGetReminder(ctx context.Context, params map[string]any) (any, error) {
	// Return cached reminder data
	reminderMutex.RLock()
	html := reminderHTML
	reminderMutex.RUnlock()

	if html == "" {
		return nil, fmt.Errorf("reminder not available")
	}

	// For now return the HTML, could parse and return structured data
	return map[string]any{
		"html": html,
	}, nil
}
