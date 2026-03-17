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

// SeedData holds content from other building blocks for seeding discussion
// threads. Populated via callbacks wired in main.go to avoid cross-block imports.
type SeedData struct {
	Title   string
	Summary string // short excerpt for the thread body
	Link    string // link to the full content
}

// GetOpinionSeed returns today's opinion post data for seeding a discussion thread.
// Wired in main.go: social.GetOpinionSeed = func() *social.SeedData { ... }
var GetOpinionSeed func() *SeedData

// GetDigestSeed returns today's news digest data for seeding a discussion thread.
// Wired in main.go: social.GetDigestSeed = func() *social.SeedData { ... }
var GetDigestSeed func() *SeedData

// StartSeeding begins the background seeding of social discussions.
// Seeds three daily threads: reminder, opinion, and digest.
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
	seedOpinion()
	seedDigest()
}

// seedReminder creates a daily discussion thread from the Islamic reminder.
func seedReminder() {
	today := todayKey()
	seedID := "reminder-" + today

	if threadExists(seedID) {
		return
	}

	rd := reminder.GetDailyReminderData()
	if rd == nil {
		return
	}

	dateParam := time.Now().Format("2006-01-02")
	reminderLink := "/reminder?date=" + dateParam

	var sb strings.Builder
	if rd.Message != "" {
		sb.WriteString(rd.Message)
		sb.WriteString("\n\n")
	}
	sb.WriteString("[Read the full reminder](" + reminderLink + ")")
	sb.WriteString("\n\n")
	sb.WriteString("*Share your reflections and thoughts on today's reminder.*")

	content := sb.String()
	if content == "" {
		return
	}

	thread := &Thread{
		ID:        seedID,
		Title:     "Daily Reminder — " + time.Now().Format("2 Jan 2006"),
		Link:      reminderLink,
		Content:   content,
		Topic:     "Islam",
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	AddSeededThread(thread)
	app.Log("social", "Seeded daily reminder thread")
}

// seedOpinion creates a daily discussion thread linked to the blog opinion post.
func seedOpinion() {
	if GetOpinionSeed == nil {
		return
	}

	today := todayKey()
	seedID := "opinion-" + today

	if threadExists(seedID) {
		return
	}

	sd := GetOpinionSeed()
	if sd == nil {
		return
	}

	var sb strings.Builder
	sb.WriteString(sd.Summary)
	sb.WriteString("\n\n")
	sb.WriteString("[Read the full opinion](" + sd.Link + ")")
	sb.WriteString("\n\n")
	sb.WriteString("*What do you think? Share your perspective.*")

	thread := &Thread{
		ID:        seedID,
		Title:     "Daily Opinion — " + time.Now().Format("2 Jan 2006"),
		Link:      sd.Link,
		Content:   sb.String(),
		Topic:     "Opinion",
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	AddSeededThread(thread)
	app.Log("social", "Seeded daily opinion thread")
}

// seedDigest creates a daily discussion thread linked to the news digest.
func seedDigest() {
	if GetDigestSeed == nil {
		return
	}

	today := todayKey()
	seedID := "digest-" + today

	if threadExists(seedID) {
		return
	}

	sd := GetDigestSeed()
	if sd == nil {
		return
	}

	var sb strings.Builder
	sb.WriteString(sd.Summary)
	sb.WriteString("\n\n")
	sb.WriteString("[Read the full digest](" + sd.Link + ")")
	sb.WriteString("\n\n")
	sb.WriteString("*Discuss what's happening today.*")

	thread := &Thread{
		ID:        seedID,
		Title:     "Daily Digest — " + time.Now().Format("2 Jan 2006"),
		Link:      sd.Link,
		Content:   sb.String(),
		Topic:     "News",
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	AddSeededThread(thread)
	app.Log("social", "Seeded daily digest thread")
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
