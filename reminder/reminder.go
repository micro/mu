package reminder

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/app"
	"mu/data"
)

// ReminderData represents the cached reminder data
type ReminderData struct {
	Verse                 string                 `json:"verse"`
	Name                  string                 `json:"name"`
	Hadith                string                 `json:"hadith"`
	Message               string                 `json:"message"`
	Updated               string                 `json:"updated"`
	Links                 map[string]interface{} `json:"links"`
	Context               string                 `json:"context"`                // AI-generated context
	ContextRequestedAt    int64                  `json:"context_requested_at"`   // Last time we requested context generation
	ContextAttempts       int                    `json:"context_attempts"`       // Number of times we've requested context
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

// handleJSON returns reminder data as JSON
func handleJSON(w http.ResponseWriter, r *http.Request) {
	reminderData := getReminderData()
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
	reminderData := getReminderData()
	if reminderData == nil {
		app.Respond(w, r, app.Response{
			Title:       "Daily Reminder",
			Description: "Islamic daily reminder",
			HTML:        `<div class="reminder-page"><p>Reminder not available at this time.</p></div>`,
		})
		return
	}

	body := generateReminderPage(reminderData)

	app.Respond(w, r, app.Response{
		Title:       "Daily Reminder",
		Description: "Daily Islamic reminder with verse, hadith, and name of Allah",
		HTML:        body,
	})
}

// getReminderData loads the cached reminder data
func getReminderData() *ReminderData {
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

// generateReminderPage generates the full reminder page HTML
func generateReminderPage(data *ReminderData) string {
	var sb strings.Builder

	sb.WriteString(`<div class="reminder-page">`)

	// Page header
	sb.WriteString(`<p class="reminder-date">`)
	if data.Updated != "" {
		sb.WriteString(fmt.Sprintf("Updated: %s", data.Updated))
	}
	sb.WriteString(`</p>`)

	// Name of Allah section
	if data.Name != "" {
		sb.WriteString(`<div class="reminder-section">`)
		sb.WriteString(`<h2>Name of Allah</h2>`)
		sb.WriteString(`<div class="reminder-content name-content">`)
		sb.WriteString(data.Name)
		sb.WriteString(`</div>`)
		
		// Add link to explore more names if available
		if data.Links != nil {
			if nameLink, ok := data.Links["name"].(string); ok && nameLink != "" {
				sb.WriteString(fmt.Sprintf(`<p class="reminder-link">%s</p>`, 
					app.Link("Explore more names", "https://reminder.dev"+nameLink)))
			}
		}
		sb.WriteString(`</div>`)
	}

	// Verse section
	if data.Verse != "" {
		sb.WriteString(`<div class="reminder-section">`)
		sb.WriteString(`<h2>Quranic Verse</h2>`)
		sb.WriteString(`<div class="reminder-content verse-content">`)
		sb.WriteString(data.Verse)
		sb.WriteString(`</div>`)
		
		// Add link to full verse context if available
		if data.Links != nil {
			if verseLink, ok := data.Links["verse"].(string); ok && verseLink != "" {
				sb.WriteString(fmt.Sprintf(`<p class="reminder-link">%s</p>`, 
					app.Link("Read full verse context", "https://reminder.dev"+verseLink)))
			}
		}
		sb.WriteString(`</div>`)
	}

	// Hadith section
	if data.Hadith != "" {
		sb.WriteString(`<div class="reminder-section">`)
		sb.WriteString(`<h2>Hadith</h2>`)
		sb.WriteString(`<div class="reminder-content hadith-content">`)
		sb.WriteString(data.Hadith)
		sb.WriteString(`</div>`)
		
		// Add link to hadith source if available
		if data.Links != nil {
			if hadithLink, ok := data.Links["hadith"].(string); ok && hadithLink != "" {
				sb.WriteString(fmt.Sprintf(`<p class="reminder-link">%s</p>`, 
					app.Link("Read more hadiths", "https://reminder.dev"+hadithLink)))
			}
		}
		sb.WriteString(`</div>`)
	}

	// Additional context/message if available
	if data.Context != "" {
		sb.WriteString(`<div class="reminder-section">`)
		sb.WriteString(`<h2>Context</h2>`)
		sb.WriteString(`<div class="reminder-content message-content">`)
		sb.WriteString(data.Context)
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	} else if data.Message != "" {
		// Fallback to message if context is not available
		sb.WriteString(`<div class="reminder-section">`)
		sb.WriteString(`<h2>Context</h2>`)
		sb.WriteString(`<div class="reminder-content message-content">`)
		sb.WriteString(data.Message)
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	}

	// Discussion section - link to chat
	sb.WriteString(`<div class="reminder-section">`)
	sb.WriteString(`<h2>Discuss</h2>`)
	sb.WriteString(`<p class="reminder-discussion">`)
	sb.WriteString(`Have questions or reflections about this reminder? `)
	sb.WriteString(app.Link("Discuss with AI", "/chat?room=reminder_daily"))
	sb.WriteString(`</p>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`</div>`)

	return sb.String()
}

// Init initializes the reminder package
func Init() {
	// Subscribe to context generation events
	contextSub := data.Subscribe(data.EventSummaryGenerated)
	go func() {
		for event := range contextSub.Chan {
			eventType, okType := event.Data["type"].(string)
			
			if okType && eventType == "reminder" {
				summary, okSummary := event.Data["summary"].(string)
				
				if okSummary {
					app.Log("reminder", "Received generated context")
					
					// Load current reminder data
					reminderData := getReminderData()
					if reminderData != nil {
						// Update with context
						reminderData.Context = summary
						
						// Save updated reminder data
						b, err := json.Marshal(reminderData)
						if err == nil {
							data.SaveFile("reminder.json", string(b))
							app.Log("reminder", "Updated reminder with generated context")
						} else {
							app.Log("reminder", "Error marshaling reminder data: %v", err)
						}
					}
				}
			}
		}
	}()
}

// RequestReminderContext requests AI-generated context for reminder data
func RequestReminderContext(reminderData *ReminderData) {
	// Skip if we already have context
	if reminderData.Context != "" {
		return
	}

	// Prepare content for context generation
	var contentParts []string
	
	if reminderData.Name != "" {
		contentParts = append(contentParts, fmt.Sprintf("Name of Allah: %s", reminderData.Name))
	}
	if reminderData.Verse != "" {
		contentParts = append(contentParts, fmt.Sprintf("Quranic Verse: %s", reminderData.Verse))
	}
	if reminderData.Hadith != "" {
		contentParts = append(contentParts, fmt.Sprintf("Hadith: %s", reminderData.Hadith))
	}
	
	contentToSummarize := strings.Join(contentParts, "\n\n")

	// Skip if there's not enough content
	if len(contentToSummarize) < 50 {
		return
	}

	// Update request tracking
	reminderData.ContextRequestedAt = time.Now().UnixNano()
	reminderData.ContextAttempts++
	
	// Save updated tracking
	b, err := json.Marshal(reminderData)
	if err == nil {
		data.SaveFile("reminder.json", string(b))
	}

	app.Log("reminder", "Requesting context generation (attempt %d)", reminderData.ContextAttempts)

	// Publish context generation request with specific prompt
	data.Publish(data.Event{
		Type: data.EventGenerateSummary,
		Data: map[string]interface{}{
			"uri":     "reminder",
			"content": contentToSummarize,
			"type":    "reminder",
		},
	})
}
