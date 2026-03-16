package social

import (
	"strings"
	"time"

	"mu/internal/app"
	"mu/reminder"
)

// SeedingEnabled controls whether daily discussion threads are created.
// Disabled by default — enable via main.go when there's an active community.
var SeedingEnabled = false

// StartSeeding begins the background seeding of social discussions.
// Currently only seeds a daily reminder thread when enabled.
func StartSeeding() {
	if !SeedingEnabled {
		app.Log("social", "Seeding disabled — set social.SeedingEnabled = true to enable")
		return
	}
	go seedLoop()
}

func seedLoop() {
	time.Sleep(30 * time.Second)

	seedAll()

	for {
		time.Sleep(time.Hour)
		seedAll()
	}
}

func seedAll() {
	seedReminder()
}

// seedReminder creates a daily discussion thread from the Islamic reminder.
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

	AddSeededThread(thread)
	app.Log("social", "Seeded daily reminder thread")
}

// AddSeededThread adds a thread without requiring auth or quota.
func AddSeededThread(thread *Thread) {
	mutex.Lock()
	threads = append([]*Thread{thread}, threads...)
	mutex.Unlock()

	save()
	indexThread(thread)
	updateCache()
}

func threadExists(id string) bool {
	mutex.RLock()
	defer mutex.RUnlock()
	return getThread(id) != nil
}

func todayKey() string {
	return time.Now().Format("2006-01-02")
}
