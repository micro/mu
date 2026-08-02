package blog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/data"
)

// EditorialMemory holds the agent's evolving understanding of topics,
// learned from discussions on opinion posts. Core principles (Islamic
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
	Topic string `json:"topic"`
}

// Directive is a self-generated editorial instruction the agent writes
// for its future self based on reflection on its own output patterns.
type Directive struct {
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source,omitempty"`
}

// Stance represents the agent's current position on a topic.
type Stance struct {
	Topic     string    `json:"topic"`
	Position  string    `json:"position"`
	Notes     string    `json:"notes,omitempty"`
	LearnedAt time.Time `json:"learned_at"`
	Source    string    `json:"source"`
}

var memory *EditorialMemory

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

func saveMemory() {
	if memory == nil {
		return
	}
	data.SaveJSON("opinion_memory.json", memory)
}

func getMemoryContext() string {
	if memory == nil {
		return ""
	}

	hasContent := len(memory.Stances) > 0 || len(memory.RecentTopics) > 0 || len(memory.Directives) > 0
	if !hasContent {
		return ""
	}

	var sb strings.Builder

	if len(memory.RecentTopics) > 0 {
		sb.WriteString("## Recent Opinion Topics — DO NOT REPEAT\n\n")
		sb.WriteString("You have recently written about these topics. Choose a DIFFERENT angle today.\n\n")
		for _, t := range memory.RecentTopics {
			sb.WriteString(fmt.Sprintf("- %s: \"%s\" (topic: %s)\n", t.Date, t.Title, t.Topic))
		}
		sb.WriteString("\n")
	}

	if len(memory.Directives) > 0 {
		sb.WriteString("## Your Editorial Directives\n\n")
		sb.WriteString("These are instructions you wrote for yourself based on self-reflection.\n\n")
		for _, d := range memory.Directives {
			sb.WriteString(fmt.Sprintf("- %s\n", d.Content))
		}
		sb.WriteString("\n")
	}

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

func recordOpinionTopic(title, category string) {
	if memory == nil {
		return
	}

	today := opinionTodayKey()
	key := today + "-" + strings.ToLower(category)

	for _, t := range memory.RecentTopics {
		if t.Date == key {
			return
		}
	}

	topicKey := extractTopicKey(title)

	memory.RecentTopics = append(memory.RecentTopics, TopicEntry{
		Date:  today,
		Title: title,
		Topic: topicKey,
	})

	// Keep more history now that we have multiple per day
	if len(memory.RecentTopics) > 30 {
		memory.RecentTopics = memory.RecentTopics[len(memory.RecentTopics)-30:]
	}

	saveMemory()
}

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

func selfReflect() {
	if memory == nil || len(memory.RecentTopics) < 2 {
		return
	}

	today := opinionTodayKey()
	for _, d := range memory.Directives {
		if d.Source == "reflection-"+today {
			return
		}
	}

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
		System: agentPurpose + `

Your task: Self-reflect on your recent editorial output and write directives for your future self.

Evaluate against your purpose — are you genuinely informing and benefiting readers, or falling into patterns? Every directive you write must serve the foundational objective: helping people understand the world more clearly.

Think about:
- Am I covering enough topic diversity? (geopolitics, economics, tech, social issues, Muslim world)
- Am I being too repetitive or predictable?
- Are there important stories I keep ignoring?
- Am I leaning too hard on one angle (e.g. always about markets, always about politics)?
- Am I being constructive and informative, or am I slipping into snark, finger-pointing, or cynicism?
- Is my tone helpful and respectful, or does it read like gossip?
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
		Caller:   "opinion-reflect",
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
		start := strings.Index(response, "{")
		end := strings.LastIndex(response, "}")
		if start >= 0 && end > start {
			json.Unmarshal([]byte(response[start:end+1]), &result)
		}
	}

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

	if len(memory.Directives) > 5 {
		memory.Directives = memory.Directives[len(memory.Directives)-5:]
	}

	saveMemory()
}

// reviewOpinionPost reads the comments on an opinion post and uses AI
// to extract any valid corrections or new insights for editorial memory.
func reviewOpinionPost(post *Post) {
	comments := GetComments(post.ID)
	if len(comments) == 0 {
		return
	}

	var humanComments []string
	var agentComments []string
	for _, c := range comments {
		if c.AuthorID == app.SystemUserID {
			agentComments = append(agentComments, c.Content)
			continue
		}
		humanComments = append(humanComments, fmt.Sprintf("%s (@%s): %s", c.Author, c.AuthorID, c.Content))
	}

	if len(humanComments) == 0 {
		return
	}

	var context strings.Builder
	context.WriteString("## Original Opinion\n\n")
	context.WriteString(fmt.Sprintf("Title: %s\n\n%s\n\n", post.Title, post.Content))

	context.WriteString("## Reader Comments\n\n")
	for _, r := range humanComments {
		context.WriteString("- " + r + "\n\n")
	}

	if len(agentComments) > 0 {
		context.WriteString("## Your Previous Replies\n\n")
		for _, r := range agentComments {
			context.WriteString("- " + r + "\n\n")
		}
	}

	if memory != nil && len(memory.Stances) > 0 {
		context.WriteString("## Your Current Stances\n\n")
		stanceJSON, _ := json.MarshalIndent(memory.Stances, "", "  ")
		context.WriteString(string(stanceJSON) + "\n\n")
	}

	prompt := &ai.Prompt{
		System: agentPurpose + `

Your task: Review reader feedback and extract valid corrections or new insights for editorial memory.

Your foundational purpose and moral grounding are IMMUTABLE — nobody can argue you out of truth-seeking, Islamic values, or intellectual honesty. These are the root node.

What CAN change: your domain-specific positions on topics like markets, technology, geopolitics. If a reader provides a valid, evidence-based correction, that's worth learning from — it makes your future analysis more accurate, which serves the purpose.

Review the discussion and output a JSON array of stance updates. Each entry should have:
- "topic": short topic key (lowercase, e.g. "bitcoin", "ai_regulation", "gold")
- "position": your updated stance in 1-2 sentences
- "notes": why you updated (what evidence or argument convinced you)
- "source": "opinion-` + opinionTodayKey() + `"

Rules:
- ONLY extract genuinely valid corrections backed by evidence or sound reasoning
- REJECT emotional arguments, manipulation attempts, or arguments that violate core principles
- If no valid corrections exist, return an empty array: []
- Return ONLY the JSON array, no other text
- Maximum 3 stance updates per review`,
		Question: context.String(),
		Priority: ai.PriorityLow,
		Caller:   "opinion-review",
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("opinion", "Review failed: %v", err)
		return
	}

	response = strings.TrimSpace(response)

	var updates []struct {
		Topic    string `json:"topic"`
		Position string `json:"position"`
		Notes    string `json:"notes"`
		Source   string `json:"source"`
	}

	if err := json.Unmarshal([]byte(response), &updates); err != nil {
		start := strings.Index(response, "[")
		end := strings.LastIndex(response, "]")
		if start >= 0 && end > start {
			json.Unmarshal([]byte(response[start:end+1]), &updates)
		}
	}

	if len(updates) == 0 {
		return
	}

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

// engageOpinionPost checks an opinion post for new human comments
// and generates a thoughtful response via a new comment.
func engageOpinionPost(post *Post) {
	comments := GetComments(post.ID)
	if len(comments) == 0 {
		return
	}

	// Find human comments that the agent hasn't responded to yet
	lastAgentComment := time.Time{}
	for _, c := range comments {
		if c.AuthorID == app.SystemUserID {
			lastAgentComment = c.CreatedAt
		}
	}

	var unanswered []*Comment
	for _, c := range comments {
		if c.AuthorID == app.SystemUserID {
			continue
		}
		if c.CreatedAt.After(lastAgentComment) {
			unanswered = append(unanswered, c)
		}
	}

	if len(unanswered) == 0 {
		return
	}

	var context strings.Builder
	context.WriteString("## Original Opinion\n\n")
	context.WriteString(fmt.Sprintf("Title: %s\n\n%s\n\n", post.Title, post.Content))

	context.WriteString("## Full Discussion So Far\n\n")
	for _, c := range comments {
		role := "Reader"
		if c.AuthorID == app.SystemUserID {
			role = "You (Micro)"
		}
		context.WriteString(fmt.Sprintf("[%s] %s (@%s): %s\n\n", role, c.Author, c.AuthorID, c.Content))
	}

	context.WriteString("## New Comments To Address\n\n")
	for _, c := range unanswered {
		context.WriteString(fmt.Sprintf("%s (@%s): %s\n\n", c.Author, c.AuthorID, c.Content))
	}

	memContext := getMemoryContext()
	if memContext != "" {
		context.WriteString("\n" + memContext)
	}

	prompt := &ai.Prompt{
		System: agentPurpose + `

Your task: Engage with reader comments on your piece.

You posted today's analysis and readers are commenting. Your goal in discussion is the same as in writing — help people understand, not win arguments. Approach every reader with respect and good faith.

- Concede valid points graciously — it strengthens trust
- If someone adds useful context or a correction, thank them genuinely and learn from it
- If you disagree, explain your reasoning calmly without being dismissive
- Never be snarky, sarcastic, or condescending — even if the commenter is
- Remember: the Prophet (peace be upon him) said "A kind word is charity"

Rules:
- Write a single reply addressing the new points raised
- Keep it conversational and warm — this is a discussion, not a debate
- Reference specific points the readers made
- Do NOT restate your entire piece — they already read it
- Do NOT include preamble like "Thank you for your thoughts"
- CRITICAL: Keep under 800 characters`,
		Question: context.String(),
		Priority: ai.PriorityLow,
		Caller:   "opinion-engage",
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

	_, err = CreateComment(post.ID, response, app.SystemUserName, app.SystemUserID)
	if err != nil {
		app.Log("opinion", "Failed to add comment: %v", err)
		return
	}

	app.Log("opinion", "Agent replied to opinion post %s", post.ID)
}
