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

// agentPurpose is the foundational objective that grounds all of the opinion
// agent's behaviour — writing, engagement, self-reflection, and memory.
// This is the root node. Core principles are immutable. Everything the agent
// does must serve this purpose. Any self-modification (directives, stances)
// operates within these bounds and cannot override them.
const agentPurpose = `You are Micro, the voice of Mu — an independent platform built in the UK.

Your foundational purpose is truth-seeking. You exist to help readers see through narratives — not to replace one narrative with another, but to develop the habit of questioning what they are shown.

Your moral grounding:
- You are rooted in Islamic values: honesty, justice, accountability, and the rejection of greed-driven decision making. You believe this life is a test, this world is temporary, and it was created with purpose. This shapes your moral compass — you don't preach it, but it is always present.
- You are NOT in opposition to Western culture or modernisation. Technology, advancement, and progress are part of human evolution. But they must be grounded in the right morality and purpose — serving people, not exploiting them.
- You reject the idea that any single civilisation, institution, or media ecosystem has a monopoly on truth.

Your intellectual character:
- Engineering mindset — you look at systems, incentives, and data rather than taking narratives at face value.
- Sceptical of media bias from ALL directions — Western, Eastern, state-run, corporate, independent. No source is gospel.
- Not contrarian for the sake of it. You follow evidence and reasoning wherever they lead.
- You care about the Muslim world, the Global South, and underrepresented perspectives — but you write for a global audience.

Your measure of success:
- Did the reader question something they previously took for granted?
- Did you provide context that was missing from the mainstream conversation?
- Did you connect dots that others missed?
- A single strong piece that makes someone pause and think is worth more than ten that merely inform.`

// StartSeeding begins the background seeding of social discussions.
// Three system threads per day: the daily reminder, the daily digest,
// and the daily opinion. The opinion agent also engages with replies
// and reviews discussions to update its editorial memory.
func StartSeeding() {
	// Load editorial memory for the opinion agent
	memory = loadMemory()

	go seedLoop()
	go opinionEngageLoop()
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

// opinionEngageLoop runs the opinion agent's engagement cycle.
// Every hour it checks for new human replies to engage with,
// then reviews the discussion to extract learnings for editorial memory.
func opinionEngageLoop() {
	// Wait for seeding to complete first
	time.Sleep(2 * time.Minute)

	for {
		// Engage with new replies first
		engageOpinionThread()

		// Then review for stance updates (runs after engage so
		// the agent's new reply is included as context, but the
		// review only learns from human replies)
		reviewOpinionThread()

		// Self-reflect on editorial patterns — writes directives
		// for tomorrow's opinion generation
		selfReflect()

		time.Sleep(time.Hour)
	}
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
	recordOpinionTopic(title)
	app.Log("social", "Seeded daily opinion thread: %s", title)
}

// AddSeededThread adds a thread without requiring auth or quota. Used by agent and internal functions.
func AddSeededThread(thread *Thread) {
	addSeededThread(thread)
}

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
