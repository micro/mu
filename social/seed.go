package social

import (
	"strings"
	"time"

	"mu/app"
	"mu/blog"
	"mu/news/reminder"
)

// opinionTopic is the topic used for daily opinion threads.
const opinionTopic = "World"

// StartSeeding begins the background seeding of social discussions.
// Three system threads per day: the daily reminder, the daily digest,
// and the daily opinion. Everything else comes from users.
func StartSeeding() {
	go seedLoop()
}

func seedLoop() {
	// Wait for other services to load first
	time.Sleep(30 * time.Second)

	seedAll()

	// Check once per hour in case data wasn't ready on startup
	for {
		time.Sleep(time.Hour)
		seedAll()
	}
}

func seedAll() {
	seedReminder()
	seedDigest()
	seedOpinion()
}

// seedReminder creates a daily discussion thread from the Islamic reminder
func seedReminder() {
	today := todayKey()
	seedID := "reminder-" + today

	if threadExists(seedID) {
		return
	}

	rd := reminder.GetReminderData()
	if rd == nil {
		return
	}

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
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
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

	content := digest.Content
	if len(content) > 500 {
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
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	addSeededThread(thread)
	app.Log("social", "Seeded daily digest thread")
}

// seedOpinion creates a daily opinion thread by analysing all available data,
// cross-referencing with web research, and generating a grounded opinion piece.
func seedOpinion() {
	today := todayKey()
	seedID := "opinion-" + today

	if threadExists(seedID) {
		return
	}

	title, body, err := generateOpinion()
	if err != nil {
		app.Log("social", "Opinion generation failed: %v", err)
		return
	}

	content := body + "\n\n*What's your take? Share your thoughts below.*"

	thread := &Thread{
		ID:        seedID,
		Title:     "Opinion: " + title,
		Content:   content,
		Topic:     opinionTopic,
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	addSeededThread(thread)
	app.Log("social", "Seeded daily opinion thread: %s", title)
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
