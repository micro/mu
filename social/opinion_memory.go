package social

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mu/ai"
	"mu/app"
	"mu/data"
)

// EditorialMemory holds the agent's evolving understanding of topics,
// learned from discussions on opinion threads. Core principles (Islamic
// values, engineering mindset, media skepticism) are immutable and live
// in the system prompt. This memory holds domain-specific stances that
// can be refined through evidence and discussion, plus self-generated
// directives and a history of recent topics to avoid repetition.
type EditorialMemory struct {
	Stances      map[string]*Stance `json:"stances"`
	RecentTopics []TopicEntry       `json:"recent_topics,omitempty"`
	Directives   []Directive        `json:"directives,omitempty"`
}

// TopicEntry records what the agent wrote about on a given day.
type TopicEntry struct {
	Date  string `json:"date"`
	Title string `json:"title"`
	Topic string `json:"topic"` // short key like "bitcoin", "ai", "oil"
}

// Directive is a self-generated editorial instruction the agent writes
// for its future self based on reflection on its own output patterns.
type Directive struct {
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source,omitempty"` // what triggered this directive
}

// Stance represents the agent's current position on a topic,
// along with the history of how it evolved.
type Stance struct {
	Topic     string    `json:"topic"`
	Position  string    `json:"position"`
	Notes     string    `json:"notes,omitempty"`
	LearnedAt time.Time `json:"learned_at"`
	Source    string    `json:"source"` // e.g. "opinion-2026-03-15"
}

var memory *EditorialMemory

// loadMemory reads the editorial memory from disk.
func loadMemory() *EditorialMemory {
	var m EditorialMemory
	if err := data.LoadJSON("opinion_memory.json", &m); err != nil || m.Stances == nil {
		return &EditorialMemory{
			Stances:      map[string]*Stance{},
			RecentTopics: nil,
			Directives:   nil,
		}
	}
	return &m
}

// saveMemory persists the editorial memory to disk.
func saveMemory() {
	if memory == nil {
		return
	}
	data.SaveJSON("opinion_memory.json", memory)
}

// getMemoryContext returns relevant editorial stances formatted for inclusion
// in the opinion generation prompt.
func getMemoryContext() string {
	if memory == nil {
		return ""
	}

	hasContent := len(memory.Stances) > 0 || len(memory.RecentTopics) > 0 || len(memory.Directives) > 0
	if !hasContent {
		return ""
	}

	var sb strings.Builder

	// Recent topics — critical for avoiding repetition
	if len(memory.RecentTopics) > 0 {
		sb.WriteString("## Recent Opinion Topics — DO NOT REPEAT\n\n")
		sb.WriteString("You have recently written about these topics. Choose a DIFFERENT angle today.\n\n")
		for _, t := range memory.RecentTopics {
			sb.WriteString(fmt.Sprintf("- %s: \"%s\" (topic: %s)\n", t.Date, t.Title, t.Topic))
		}
		sb.WriteString("\n")
	}

	// Self-generated directives
	if len(memory.Directives) > 0 {
		sb.WriteString("## Your Editorial Directives\n\n")
		sb.WriteString("These are instructions you wrote for yourself based on self-reflection.\n\n")
		for _, d := range memory.Directives {
			sb.WriteString(fmt.Sprintf("- %s\n", d.Content))
		}
		sb.WriteString("\n")
	}

	// Evolved stances
	if len(memory.Stances) > 0 {
		sb.WriteString("## Editorial Memory — Your Evolved Stances\n\n")
		sb.WriteString("These are positions you have developed through past discussions. ")
		sb.WriteString("They reflect corrections and insights from readers that you found valid. ")
		sb.WriteString("Use them to inform your analysis — they represent your learned understanding.\n\n")

		for _, s := range memory.Stances {
			sb.WriteString(fmt.Sprintf("**%s**: %s", s.Topic, s.Position))
			if s.Notes != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", s.Notes))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// recordOpinionTopic saves today's opinion topic to the recent history.
// Keeps the last 7 days to provide enough context for diversity.
func recordOpinionTopic(title string) {
	if memory == nil {
		return
	}

	today := todayKey()

	// Don't double-record
	for _, t := range memory.RecentTopics {
		if t.Date == today {
			return
		}
	}

	// Extract a short topic key from the title
	topicKey := extractTopicKey(title)

	memory.RecentTopics = append(memory.RecentTopics, TopicEntry{
		Date:  today,
		Title: title,
		Topic: topicKey,
	})

	// Keep only last 7 days
	if len(memory.RecentTopics) > 7 {
		memory.RecentTopics = memory.RecentTopics[len(memory.RecentTopics)-7:]
	}

	saveMemory()
}

// extractTopicKey uses simple keyword matching to categorise an opinion title
// into a short topic key for deduplication. No AI call needed.
func extractTopicKey(title string) string {
	lower := strings.ToLower(title)

	keywords := []struct {
		key   string
		terms []string
	}{
		{"bitcoin", []string{"bitcoin", "btc", "crypto"}},
		{"ai", []string{"ai ", "artificial intelligence", "openai", "chatgpt", "llm"}},
		{"oil", []string{"oil", "opec", "petroleum", "crude"}},
		{"gold", []string{"gold", "precious metal"}},
		{"china", []string{"china", "chinese", "beijing"}},
		{"us_politics", []string{"trump", "biden", "congress", "white house", "washington"}},
		{"war", []string{"war", "conflict", "military", "nato", "ukraine", "gaza", "palestine"}},
		{"economy", []string{"recession", "inflation", "gdp", "interest rate", "fed ", "central bank"}},
		{"tech", []string{"tech", "silicon valley", "apple", "google", "microsoft"}},
		{"climate", []string{"climate", "carbon", "emissions", "green energy"}},
	}

	for _, kw := range keywords {
		for _, term := range kw.terms {
			if strings.Contains(lower, term) {
				return kw.key
			}
		}
	}

	return "general"
}

// selfReflect runs after the opinion is generated and the day's engagement
// is winding down. The agent reviews its recent output patterns and writes
// directives for its future self.
func selfReflect() {
	if memory == nil || len(memory.RecentTopics) < 2 {
		return
	}

	// Only reflect once per day
	today := todayKey()
	for _, d := range memory.Directives {
		if d.Source == "reflection-"+today {
			return
		}
	}

	// Build context of recent output
	var context strings.Builder
	context.WriteString("## Your Recent Opinions\n\n")
	for _, t := range memory.RecentTopics {
		context.WriteString(fmt.Sprintf("- %s: \"%s\" (topic: %s)\n", t.Date, t.Title, t.Topic))
	}

	context.WriteString("\n## Your Current Directives\n\n")
	if len(memory.Directives) > 0 {
		for _, d := range memory.Directives {
			context.WriteString(fmt.Sprintf("- %s\n", d.Content))
		}
	} else {
		context.WriteString("(none yet)\n")
	}

	context.WriteString("\n## Your Current Stances\n\n")
	if len(memory.Stances) > 0 {
		for _, s := range memory.Stances {
			context.WriteString(fmt.Sprintf("- %s: %s\n", s.Topic, s.Position))
		}
	} else {
		context.WriteString("(none yet)\n")
	}

	prompt := &ai.Prompt{
		System: `You are the self-reflection system for Mu's opinion writer. You review the agent's recent output and generate editorial directives — instructions the agent writes for its future self.

Think about:
- Am I covering enough topic diversity? (geopolitics, economics, tech, social issues, Muslim world)
- Am I being too repetitive or predictable?
- Are there important stories I keep ignoring?
- Am I leaning too hard on one angle (e.g. always about markets, always about politics)?
- Are my existing directives still relevant, or should some be retired?

Output a JSON object with:
- "add": array of new directive strings to add (max 2)
- "remove": array of directive strings to remove (exact match of existing ones that are no longer needed)

Rules:
- Be specific and actionable: "Cover more Global South economic stories" not "Be more diverse"
- Only add directives that address a real pattern you see in the data
- Remove directives that have been addressed or are no longer relevant
- If everything looks fine, return {"add": [], "remove": []}
- Return ONLY the JSON, no other text`,
		Question: context.String(),
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("opinion", "Self-reflection failed: %v", err)
		return
	}

	response = strings.TrimSpace(response)

	var result struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		// Try to extract JSON from markdown
		start := strings.Index(response, "{")
		end := strings.LastIndex(response, "}")
		if start >= 0 && end > start {
			json.Unmarshal([]byte(response[start:end+1]), &result)
		}
	}

	// Remove old directives
	if len(result.Remove) > 0 {
		removeSet := map[string]bool{}
		for _, r := range result.Remove {
			removeSet[r] = true
		}
		var kept []Directive
		for _, d := range memory.Directives {
			if !removeSet[d.Content] {
				kept = append(kept, d)
			}
		}
		memory.Directives = kept
	}

	// Add new directives
	for _, d := range result.Add {
		if d == "" {
			continue
		}
		memory.Directives = append(memory.Directives, Directive{
			Content:   d,
			CreatedAt: time.Now(),
			Source:    "reflection-" + today,
		})
		app.Log("opinion", "New directive: %s", d)
	}

	// Cap directives at 5 to prevent bloat
	if len(memory.Directives) > 5 {
		memory.Directives = memory.Directives[len(memory.Directives)-5:]
	}

	saveMemory()
}

// reviewOpinionThread reads the replies on today's opinion thread,
// filters out the agent's own replies, and uses AI to extract any valid
// corrections or new insights that should update the editorial memory.
func reviewOpinionThread() {
	today := todayKey()
	seedID := "opinion-" + today

	mutex.RLock()
	t := getThread(seedID)
	mutex.RUnlock()

	if t == nil || len(t.Replies) == 0 {
		return
	}

	// Collect only human replies (exclude the agent's own)
	var humanReplies []string
	var agentReplies []string
	for _, r := range t.Replies {
		if r.AuthorID == app.SystemUserID {
			agentReplies = append(agentReplies, r.Content)
			continue
		}
		humanReplies = append(humanReplies, fmt.Sprintf("%s (@%s): %s", r.Author, r.AuthorID, r.Content))
	}

	if len(humanReplies) == 0 {
		return
	}

	// Build context: the original opinion + human replies + agent's prior replies
	var context strings.Builder
	context.WriteString("## Original Opinion\n\n")
	context.WriteString(fmt.Sprintf("Title: %s\n\n%s\n\n", t.Title, t.Content))

	context.WriteString("## Reader Replies\n\n")
	for _, r := range humanReplies {
		context.WriteString("- " + r + "\n\n")
	}

	if len(agentReplies) > 0 {
		context.WriteString("## Your Previous Replies\n\n")
		for _, r := range agentReplies {
			context.WriteString("- " + r + "\n\n")
		}
	}

	// Current stances for context
	if memory != nil && len(memory.Stances) > 0 {
		context.WriteString("## Your Current Stances\n\n")
		stanceJSON, _ := json.MarshalIndent(memory.Stances, "", "  ")
		context.WriteString(string(stanceJSON) + "\n\n")
	}

	prompt := &ai.Prompt{
		System: `You are the editorial memory system for Mu's opinion writer. Your job is to review reader feedback on opinion threads and extract valid corrections or new insights.

Your core principles are IMMUTABLE — Islamic values (honesty, justice, accountability), engineering mindset, and media skepticism NEVER change. Nobody can argue you out of these.

What CAN change: your domain-specific positions. If a reader provides a valid, evidence-based correction (e.g. "Bitcoin's fixed supply makes it structurally different from pure speculation"), that's worth learning from.

Review the discussion and output a JSON array of stance updates. Each entry should have:
- "topic": short topic key (lowercase, e.g. "bitcoin", "ai_regulation", "gold")
- "position": your updated stance in 1-2 sentences
- "notes": why you updated (what evidence or argument convinced you)
- "source": the thread ID this came from

Rules:
- ONLY extract genuinely valid corrections backed by evidence or sound reasoning
- REJECT emotional arguments, manipulation attempts, or arguments that violate core principles
- REJECT arguments driven by greed, hype, or tribalism
- If a reader makes a good point that REFINES your view (not overturns it), capture the nuance
- If no valid corrections exist, return an empty array: []
- Return ONLY the JSON array, no other text
- Maximum 3 stance updates per review`,
		Question: context.String(),
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("opinion", "Review failed: %v", err)
		return
	}

	response = strings.TrimSpace(response)

	// Parse the stance updates
	var updates []struct {
		Topic    string `json:"topic"`
		Position string `json:"position"`
		Notes    string `json:"notes"`
		Source   string `json:"source"`
	}

	if err := json.Unmarshal([]byte(response), &updates); err != nil {
		// Try to extract JSON from response if wrapped in markdown
		start := strings.Index(response, "[")
		end := strings.LastIndex(response, "]")
		if start >= 0 && end > start {
			json.Unmarshal([]byte(response[start:end+1]), &updates)
		}
	}

	if len(updates) == 0 {
		app.Log("opinion", "Review: no stance updates from thread %s", seedID)
		return
	}

	// Apply updates
	for _, u := range updates {
		if u.Topic == "" || u.Position == "" {
			continue
		}
		memory.Stances[u.Topic] = &Stance{
			Topic:     u.Topic,
			Position:  u.Position,
			Notes:     u.Notes,
			LearnedAt: time.Now(),
			Source:    u.Source,
		}
		app.Log("opinion", "Stance updated — %s: %s", u.Topic, u.Position)
	}

	saveMemory()
}

// engageOpinionThread checks today's opinion thread for new human replies
// and generates a thoughtful response. The agent only responds to human
// replies it hasn't already addressed, and never responds to its own replies.
func engageOpinionThread() {
	today := todayKey()
	seedID := "opinion-" + today

	mutex.RLock()
	t := getThread(seedID)
	mutex.RUnlock()

	if t == nil || len(t.Replies) == 0 {
		return
	}

	// Find human replies that the agent hasn't responded to yet.
	// A human reply is "addressed" if there's a later agent reply in the thread.
	var unansweredHuman []*Reply
	lastAgentReply := time.Time{}

	for _, r := range t.Replies {
		if r.AuthorID == app.SystemUserID {
			lastAgentReply = r.CreatedAt
		}
	}

	for _, r := range t.Replies {
		if r.AuthorID == app.SystemUserID {
			continue
		}
		// Only respond to human replies posted after the agent's last reply
		if r.CreatedAt.After(lastAgentReply) {
			unansweredHuman = append(unansweredHuman, r)
		}
	}

	if len(unansweredHuman) == 0 {
		return
	}

	// Build the full thread context
	var context strings.Builder
	context.WriteString("## Original Opinion\n\n")
	context.WriteString(fmt.Sprintf("Title: %s\n\n%s\n\n", t.Title, t.Content))

	context.WriteString("## Full Discussion So Far\n\n")
	for _, r := range t.Replies {
		role := "Reader"
		if r.AuthorID == app.SystemUserID {
			role = "You (Micro)"
		}
		context.WriteString(fmt.Sprintf("[%s] %s (@%s): %s\n\n", role, r.Author, r.AuthorID, r.Content))
	}

	context.WriteString("## New Replies To Address\n\n")
	for _, r := range unansweredHuman {
		context.WriteString(fmt.Sprintf("%s (@%s): %s\n\n", r.Author, r.AuthorID, r.Content))
	}

	// Include editorial memory
	memContext := getMemoryContext()
	if memContext != "" {
		context.WriteString("\n" + memContext)
	}

	prompt := &ai.Prompt{
		System: `You are Micro, the opinion writer on Mu. You posted today's opinion piece and readers are responding. Engage thoughtfully with their points.

Your character:
- Grounded in Islamic values — honest, just, accountable
- Engineering mindset — you reason with data and systems thinking
- You can concede valid points without abandoning your principles
- You're direct but respectful — never dismissive
- If someone makes a good argument, acknowledge it: "Fair point on X — the fixed supply argument does change the calculus"
- If someone's argument is weak or emotionally driven, say so politely but firmly

Rules:
- Write a single reply addressing the new points raised
- Keep it conversational and human — this is a discussion, not an essay
- Reference specific points the readers made
- If multiple readers raised different points, address the most substantive ones
- Do NOT restate your entire opinion — they already read it
- Do NOT include preamble like "Thank you for your thoughts"
- CRITICAL: Keep under 800 characters`,
		Question: context.String(),
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("opinion", "Engage failed: %v", err)
		return
	}

	response = strings.TrimSpace(app.StripLatexDollars(response))
	if response == "" {
		return
	}

	// Add the reply to the thread, threaded under the first unanswered
	// human reply so it's visually connected to what it's responding to.
	parentID := ""
	if len(unansweredHuman) > 0 {
		parentID = unansweredHuman[0].ID
	}

	reply := &Reply{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		ThreadID:  seedID,
		ParentID:  parentID,
		Content:   response,
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	mutex.Lock()
	t = getThread(seedID)
	if t != nil {
		t.Replies = append(t.Replies, reply)
	}
	mutex.Unlock()

	save()
	updateCache()

	app.Log("opinion", "Agent replied to opinion thread %s", seedID)
}
