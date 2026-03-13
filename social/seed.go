package social

import (
	"strings"
	"time"

	"mu/app"
	"mu/blog"
	"mu/data"
	"mu/news/reminder"
)

// StartSeeding begins the background seeding of social discussions
func StartSeeding() {
	go seedLoop()
}

func seedLoop() {
	// Wait for other services to load first
	time.Sleep(30 * time.Second)

	// Seed immediately on startup
	seedAll()

	// Then check every hour
	for {
		time.Sleep(time.Hour)
		seedAll()
	}
}

func seedAll() {
	seedReminder()
	seedDigest()
	seedTopNews()
}

// seedReminder creates a daily discussion thread from the Islamic reminder
func seedReminder() {
	today := todayKey()
	seedID := "reminder-" + today

	// Check if already seeded today
	if threadExists(seedID) {
		return
	}

	rd := reminder.GetReminderData()
	if rd == nil {
		return
	}

	// Build the thread content with just the message summary and a link
	// to the full reminder page. Embedding the full content (verse, hadith, name)
	// causes markdown formatting issues (backticks become pre blocks, etc.)
	var sb strings.Builder

	if rd.Message != "" {
		sb.WriteString(rd.Message)
		sb.WriteString("\n\n")
	}

	sb.WriteString("[Read the full reminder](/reminder)")
	sb.WriteString("\n\n")
	sb.WriteString("*Share your reflections and thoughts on today's reminder.*")

	content := sb.String()
	if content == "" {
		return
	}

	thread := &Thread{
		ID:        seedID,
		Title:     "Daily Reminder — " + time.Now().Format("2 Jan 2006"),
		Link:      "/reminder",
		Content:   content,
		Topic:     "Islam",
		Author:    "Mu",
		AuthorID:  "mu",
		CreatedAt: time.Now(),
	}

	addSeededThread(thread)
	app.Log("social", "Seeded daily reminder thread")
}

// seedDigest creates a discussion thread for the daily blog digest
func seedDigest() {
	today := todayKey()
	seedID := "digest-" + today

	if threadExists(seedID) {
		return
	}

	digest := blog.FindTodayDigest()
	if digest == nil {
		return
	}

	// Create a summary for discussion — first few lines of digest
	content := digest.Content
	if len(content) > 500 {
		// Truncate at a sentence boundary
		cut := strings.LastIndex(content[:500], ". ")
		if cut > 200 {
			content = content[:cut+1]
		} else {
			content = content[:500]
		}
		content += "\n\n[Read the full digest](/post/" + digest.ID + ")"
	}

	content += "\n\n*What are your thoughts on today's top stories?*"

	thread := &Thread{
		ID:        seedID,
		Title:     "Daily Digest — " + time.Now().Format("2 Jan 2006"),
		Link:      "/post/" + digest.ID,
		Content:   content,
		Topic:     "World",
		Author:    "Mu",
		AuthorID:  "mu",
		CreatedAt: time.Now(),
	}

	addSeededThread(thread)
	app.Log("social", "Seeded daily digest thread")
}

// seedTopNews creates discussion threads from the most notable news stories
func seedTopNews() {
	today := todayKey()
	seedID := "news-" + today

	if threadExists(seedID) {
		return
	}

	// Get recent news items
	entries := data.GetByType("news", 30)
	if len(entries) == 0 {
		return
	}

	// Pick the top story — most recently indexed, which is first
	entry := entries[0]

	// Extract URL from metadata
	link := ""
	if entry.Metadata != nil {
		if u, ok := entry.Metadata["url"].(string); ok {
			link = u
		}
	}

	// Determine topic from news metadata or default to World
	topic := "World"
	if entry.Metadata != nil {
		if t, ok := entry.Metadata["topic"].(string); ok && isValidTopic(t) {
			topic = t
		}
	}

	// Use the first part of content as the discussion body
	content := entry.Content
	if len(content) > 400 {
		cut := strings.LastIndex(content[:400], ". ")
		if cut > 150 {
			content = content[:cut+1]
		} else {
			content = content[:400] + "..."
		}
	}
	content += "\n\n*What do you think about this?*"

	thread := &Thread{
		ID:        seedID,
		Title:     entry.Title,
		Link:      link,
		Content:   content,
		Topic:     topic,
		Author:    "Mu",
		AuthorID:  "mu",
		CreatedAt: time.Now(),
	}

	addSeededThread(thread)
	app.Log("social", "Seeded top news thread")
}

// addSeededThread adds a thread without requiring auth or quota
func addSeededThread(thread *Thread) {
	mutex.Lock()
	threads = append([]*Thread{thread}, threads...)
	mutex.Unlock()

	save()
	indexThread(thread)
	updateCache()
}

// threadExists checks if a thread with the given ID already exists
func threadExists(id string) bool {
	mutex.RLock()
	defer mutex.RUnlock()
	return getThread(id) != nil
}

// todayKey returns today's date as a string key
func todayKey() string {
	return time.Now().Format("2006-01-02")
}
