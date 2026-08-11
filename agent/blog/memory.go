package blog

import (
	"fmt"
	"strings"
	"time"

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
