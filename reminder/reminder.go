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

	// Save full JSON data for the reminder page
	data.SaveFile("reminder.json", string(b))

	link := "/reminder"

	html := fmt.Sprintf(`<div class="item"><div class="verse">%s</div></div>`, val["verse"])
	html += app.Link("More", link)

	reminderMutex.Lock()
	reminderHTML = html
	data.SaveFile("reminder.html", html)
	reminderMutex.Unlock()

	// Extract message and updated for indexing
	message := ""
	if m, ok := val["message"]; ok {
		message = m.(string)
	}
	updated := ""
	if u, ok := val["updated"]; ok {
		updated = u.(string)
	}

	// Index with just the message summary and a link to the full reminder page.
	// The full content (verse, hadith, name) contains markdown that doesn't render
	// well in chat threads, and it changes hourly so embedding it causes stale content.
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

// ReminderData represents the cached reminder data
type ReminderData struct {
	Verse   string                 `json:"verse"`
	Name    string                 `json:"name"`
	Hadith  string                 `json:"hadith"`
	Message string                 `json:"message"`
	Updated string                 `json:"updated"`
	Links   map[string]interface{} `json:"links"`
}

// Handler handles /reminder requests
func Handler(w http.ResponseWriter, r *http.Request) {
	// JSON response for API
	if app.WantsJSON(r) {
		handleJSON(w, r)
		return
	}

	// HTML response for browser
	handleHTML(w, r)
}

// getReminderForRequest returns the appropriate reminder data based on the
// request's ?date= query parameter. If a date is provided, fetches the fixed
// daily reminder for that date. Otherwise returns the latest (hourly) reminder.
func getReminderForRequest(r *http.Request) (rd *ReminderData, date string) {
	date = r.URL.Query().Get("date")
	if date != "" {
		// Validate date format
		if _, err := time.Parse("2006-01-02", date); err != nil {
			date = ""
		}
	}

	if date != "" {
		return GetDailyReminderForDate(date), date
	}
	return GetReminderData(), ""
}

// handleJSON returns reminder data as JSON
func handleJSON(w http.ResponseWriter, r *http.Request) {
	reminderData, _ := getReminderForRequest(r)
	if reminderData == nil {
		app.RespondJSON(w, map[string]interface{}{
			"error": "Reminder data not available",
		})
		return
	}

	app.RespondJSON(w, reminderData)
}

// handleHTML returns reminder page as HTML
func handleHTML(w http.ResponseWriter, r *http.Request) {
	reminderData, date := getReminderForRequest(r)
	if reminderData == nil {
		app.Respond(w, r, app.Response{
			Title:       "Reminder",
			Description: "Islamic reminder",
			HTML:        `<div class="reminder-page"><p>Reminder not available at this time.</p></div>`,
		})
		return
	}

	title := "Reminder"
	if date != "" {
		if t, err := time.Parse("2006-01-02", date); err == nil {
			title = "Reminder — " + t.Format("2 Jan 2006")
		}
	}

	body := generateReminderPage(reminderData)

	app.Respond(w, r, app.Response{
		Title:       title,
		Description: "Islamic reminder with verse, hadith, and name of Allah",
		HTML:        body,
	})
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

// generateReminderPage generates the full reminder page HTML.
// Message as intro text, then one card with verse, hadith, and name
// as sections separated by dividers, discuss link at the bottom.
func generateReminderPage(rd *ReminderData) string {
	var sb strings.Builder

	sb.WriteString(`<div class="reminder-page">`)

	// Message as intro text above the card
	if rd.Message != "" {
		sb.WriteString(fmt.Sprintf(`<p class="reminder-intro">%s</p>`, rd.Message))
	}

	// Single card with all content
	var content strings.Builder

	// Verse section
	if rd.Verse != "" {
		content.WriteString(`<div class="reminder-section">`)
		content.WriteString(`<h5>Verse</h5>`)
		content.WriteString(`<p class="info">From the Quran</p>`)
		content.WriteString(fmt.Sprintf(`<div class="reminder-text">%s</div>`, rd.Verse))
		if rd.Links != nil {
			if verseLink, ok := rd.Links["verse"].(string); ok && verseLink != "" {
				content.WriteString(app.Link("Continue reading", "https://reminder.dev"+verseLink))
			}
		}
		content.WriteString(`</div>`)
	}

	// Hadith section
	if rd.Hadith != "" {
		content.WriteString(`<div class="reminder-section">`)
		content.WriteString(`<h5>Hadith</h5>`)
		content.WriteString(`<p class="info">From Sahih Al Bukhari</p>`)
		content.WriteString(fmt.Sprintf(`<div class="reminder-text">%s</div>`, rd.Hadith))
		if rd.Links != nil {
			if hadithLink, ok := rd.Links["hadith"].(string); ok && hadithLink != "" {
				content.WriteString(app.Link("Read more", "https://reminder.dev"+hadithLink))
			}
		}
		content.WriteString(`</div>`)
	}

	// Name section
	if rd.Name != "" {
		content.WriteString(`<div class="reminder-section">`)
		content.WriteString(`<h5>Name</h5>`)
		content.WriteString(`<p class="info">From the names of Allah</p>`)
		content.WriteString(fmt.Sprintf(`<div class="reminder-text">%s</div>`, rd.Name))
		if rd.Links != nil {
			if nameLink, ok := rd.Links["name"].(string); ok && nameLink != "" {
				content.WriteString(app.Link("Read more", "https://reminder.dev"+nameLink))
			}
		}
		content.WriteString(`</div>`)
	}

	// Discuss link
	content.WriteString(`<div class="reminder-section reminder-section-last">`)
	content.WriteString(app.Link("Discuss this reminder", "/chat?id=reminder_daily"))
	content.WriteString(`</div>`)

	sb.WriteString(app.Card("reminder", "Reminder", content.String()))

	sb.WriteString(`</div>`)

	return sb.String()
}
