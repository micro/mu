package reminder

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/event"
)

var (
	reminderMutex sync.RWMutex
	reminderHTML  string
)

// Load initializes the reminder data
func Load() {
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

	// Save full JSON data
	data.SaveFile("reminder.json", string(b))

	verseText := fmt.Sprintf("%v", val["verse"])
	// Deduplicate header when Arabic and English names match
	// e.g. "Muhammad - Muhammad - 47:1" → "Muhammad - 47:1"
	verseText = deduplicateVerseName(verseText)
	html := fmt.Sprintf(`<div class="item"><div class="verse">%s</div></div>`, verseText)

	// Link to the specific verse on reminder.dev
	moreURL := "https://reminder.dev"
	if links, ok := val["links"].(map[string]interface{}); ok {
		if verse, ok := links["verse"].(string); ok && verse != "" {
			moreURL = "https://reminder.dev" + verse
		}
	}
	html += app.Link("More", moreURL)

	reminderMutex.Lock()
	reminderHTML = html
	data.SaveFile("reminder.html", html)
	reminderMutex.Unlock()
	event.Publish(event.Event{Type: "reminder_updated"})

	// Extract message and updated for indexing
	message := ""
	if m, ok := val["message"]; ok {
		message = m.(string)
	}
	updated := ""
	if u, ok := val["updated"]; ok {
		updated = u.(string)
	}

	// Index with just the message summary. The full content (verse, hadith, name)
	// contains markdown that doesn't render well in chat threads, and it changes
	// hourly so embedding it causes stale content.
	summary := message
	if summary == "" {
		summary = "Today's Islamic reminder is ready."
	}

	// Index with ID "daily" (not "reminder_daily") because the chat room type extraction
	// will split "reminder_daily" into type="reminder" and id="daily", then look up just "daily"
	data.Index(
		"daily",
		"reminder",
		"Daily Reminder",
		summary,
		map[string]interface{}{
			"url":     "https://reminder.dev",
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

// ReminderData represents the cached reminder data
type ReminderData struct {
	Verse   string                 `json:"verse"`
	Name    string                 `json:"name"`
	Hadith  string                 `json:"hadith"`
	Message string                 `json:"message"`
	Updated string                 `json:"updated"`
	Links   map[string]interface{} `json:"links"`
}

// Handler redirects /reminder to the current verse on reminder.dev
func Handler(w http.ResponseWriter, r *http.Request) {
	url := "https://reminder.dev"
	rd := GetReminderData()
	if rd != nil {
		if v, ok := rd.Links["verse"].(string); ok && v != "" {
			url = "https://reminder.dev" + v
		}
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// GetReminderData loads the cached reminder data (from api/latest, rotates hourly)
func GetReminderData() *ReminderData {
	// Load from cache
	b, err := data.LoadFile("reminder.json")
	if err != nil {
		app.Log("reminder", "Error loading reminder data: %v", err)
		return nil
	}

	var reminderData ReminderData
	if err := json.Unmarshal(b, &reminderData); err != nil {
		app.Log("reminder", "Error parsing reminder data: %v", err)
		return nil
	}

	return &reminderData
}

// GetDailyReminderData fetches the fixed daily reminder from reminder.dev/api/daily.
// Unlike GetReminderData (which rotates hourly), this returns the same content
// all day — suitable for seeding social threads and opinion pieces.
// Results are cached per date to avoid repeated API calls.
func GetDailyReminderData() *ReminderData {
	return GetDailyReminderForDate(time.Now().Format("2006-01-02"))
}

// GetDailyReminderForDate fetches the daily reminder for a specific date (YYYY-MM-DD).
// Results are cached per date.
func GetDailyReminderForDate(date string) *ReminderData {
	cacheFile := "reminder_daily_" + date + ".json"

	// Check cache
	b, err := data.LoadFile(cacheFile)
	if err == nil {
		var rd ReminderData
		if json.Unmarshal(b, &rd) == nil {
			return &rd
		}
	}

	// Fetch from reminder.dev/api/daily?date=YYYY-MM-DD
	url := "https://reminder.dev/api/daily"
	if date != "" {
		url += "?date=" + date
	}

	resp, err := http.Get(url)
	if err != nil {
		app.Log("reminder", "Error fetching daily reminder for %s: %v", date, err)
		// Only fall back to latest for today
		if date == time.Now().Format("2006-01-02") {
			return GetReminderData()
		}
		return nil
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var rd ReminderData
	if err := json.Unmarshal(body, &rd); err != nil {
		app.Log("reminder", "Error parsing daily reminder for %s: %v", date, err)
		return nil
	}

	// Cache
	data.SaveFile(cacheFile, string(body))
	app.Log("reminder", "Fetched daily reminder for %s", date)
	return &rd
}

// deduplicateVerseName fixes the header line when Arabic and English names
// are identical, e.g. "Muhammad - Muhammad - 47:1" → "Muhammad - 47:1"
// or "Luqman - Luqman - 31:3" → "Luqman - 31:3"
func deduplicateVerseName(text string) string {
	// Header is the first line, before any newline
	firstNewline := strings.Index(text, "\n")
	if firstNewline < 0 {
		firstNewline = len(text)
	}
	header := text[:firstNewline]
	rest := text[firstNewline:]

	// Format is "{Arabic} - {English} - {Chapter}:{Verse}"
	parts := strings.SplitN(header, " - ", 3)
	if len(parts) == 3 && strings.EqualFold(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])) {
		header = parts[0] + " - " + parts[2]
	}

	return header + rest
}

