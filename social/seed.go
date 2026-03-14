package social

import (
	"fmt"
	"strings"
	"time"

	"mu/ai"
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
	seedTopicThreads()
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

// seedTopicThreads creates one "Daily [Topic]" thread per news category.
// Each thread summarises the top stories for that topic, giving users
// a focused place to discuss without flooding the feed with individual articles.
func seedTopicThreads() {
	today := todayKey()
	dateLabel := time.Now().Format("2 Jan 2006")

	entries := data.GetByType("news", 50)
	if len(entries) == 0 {
		return
	}

	// Group entries by category
	byCategory := map[string][]*data.IndexEntry{}
	for _, entry := range entries {
		cat := "World"
		if entry.Metadata != nil {
			if c, ok := entry.Metadata["category"].(string); ok && c != "" {
				cat = c
			}
		}
		byCategory[cat] = append(byCategory[cat], entry)
	}

	// Create one daily thread per topic that has stories
	for _, topic := range topics {
		if strings.EqualFold(topic, "all") {
			continue
		}

		entries := byCategory[topic]
		if len(entries) == 0 {
			continue
		}

		seedID := fmt.Sprintf("daily-%s-%s", strings.ToLower(topic), today)
		if threadExists(seedID) {
			continue
		}

		// Build a summary of the top stories for this topic
		content := buildTopicSummary(topic, entries)
		if content == "" {
			continue
		}

		thread := &Thread{
			ID:        seedID,
			Title:     fmt.Sprintf("Daily %s — %s", topic, dateLabel),
			Link:      "/news#" + topic,
			Content:   content,
			Topic:     topic,
			Author:    app.SystemUserName,
			AuthorID:  app.SystemUserID,
			CreatedAt: time.Now(),
		}

		addSeededThread(thread)
		app.Log("social", "Seeded daily %s thread (%d stories)", topic, len(entries))
	}
}

// buildTopicSummary creates a discussion-ready summary of a topic's stories.
// Lists the headlines and uses AI to generate a short contextual overview
// connecting the stories and highlighting what matters.
func buildTopicSummary(topic string, entries []*data.IndexEntry) string {
	// Cap to top 5 stories per topic
	if len(entries) > 5 {
		entries = entries[:5]
	}

	// Build headline list for the AI
	var headlines []string
	var headlineLinks []string
	for _, e := range entries {
		headlines = append(headlines, e.Title)
		url := ""
		if e.Metadata != nil {
			if u, ok := e.Metadata["url"].(string); ok {
				url = u
			}
		}
		if url != "" {
			headlineLinks = append(headlineLinks, fmt.Sprintf("- [%s](%s)", e.Title, url))
		} else {
			headlineLinks = append(headlineLinks, fmt.Sprintf("- %s", e.Title))
		}
	}

	// Use AI to write a brief overview connecting the stories
	overview := generateTopicOverview(topic, headlines)

	var sb strings.Builder
	if overview != "" {
		sb.WriteString(overview)
		sb.WriteString("\n\n")
	}

	sb.WriteString("**Today's stories:**\n")
	for _, link := range headlineLinks {
		sb.WriteString(link + "\n")
	}

	sb.WriteString(fmt.Sprintf("\n[See all %s news](/news#%s)", topic, topic))
	sb.WriteString("\n\n*What caught your attention? Share your thoughts.*")

	return sb.String()
}

// generateTopicOverview asks AI to write a brief contextual overview
// connecting the day's stories for a given topic.
func generateTopicOverview(topic string, headlines []string) string {
	if len(headlines) == 0 {
		return ""
	}

	prompt := &ai.Prompt{
		System: fmt.Sprintf(`You are writing a brief daily overview for the "%s" section of Mu, an independent truth-seeking platform.

You will receive today's headlines. Write 1-2 sentences connecting the key themes or highlighting the most significant development. This frames the discussion for the day.

Rules:
- Be direct and factual — no preamble, no "Today in %s..."
- Name countries, companies, and people explicitly
- Globally neutral — no US-centric framing
- Where relevant, note impacts on the Muslim world or marginalised communities
- Write dollar amounts as plain numbers like $100
- CRITICAL: Keep under 300 characters`, topic, topic),
		Question: "Today's headlines:\n\n" + strings.Join(headlines, "\n"),
		Priority: ai.PriorityLow,
	}

	resp, err := ai.Ask(prompt)
	if err != nil {
		app.Log("social", "AI topic overview failed for %s: %v", topic, err)
		return ""
	}

	return strings.TrimSpace(app.StripLatexDollars(resp))
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
