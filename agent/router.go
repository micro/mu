package agent

import (
	"strings"

	"mu/ai"
)

// IntentType represents the classified intent of a user request
type IntentType string

const (
	IntentTask     IntentType = "task"     // Execute a task with tools
	IntentIslamic  IntentType = "islamic"  // Islamic/religious question
	IntentArabic   IntentType = "arabic"   // Arabic/cultural question
	IntentCoding   IntentType = "coding"   // Build/create apps
	IntentNews     IntentType = "news"     // Search news
	IntentGeneral  IntentType = "general"  // General question (redirect to chat)
	IntentUnknown  IntentType = "unknown"  // Needs LLM to classify
)

// Intent holds the classified intent and suggested provider
type Intent struct {
	Type     IntentType
	Provider string // ai.ProviderAnthropic, ai.ProviderFanar, etc.
	Tool     string // Suggested tool if known
}

// ClassifyIntent uses rules-based matching first, then LLM fallback
func ClassifyIntent(input string) *Intent {
	lower := strings.ToLower(input)
	
	// Layer 1: Rules-based classification (fast)
	
	// Meta: list tools/capabilities
	if containsAny(lower, []string{"list tools", "what tools", "available tools", "show tools", "what can you do", "capabilities", "help me"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "tools.list"}
	}
	
	// Action-driven: system does something in background (flows)
	// Keywords: remind, alert, notify, send me, every day/morning, schedule
	if containsAny(lower, []string{"remind me", "alert me", "notify me", "send me", "email me", "tell me when", "let me know when"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "flow.create"}
	}
	if containsAny(lower, []string{"every day", "every morning", "every hour", "every evening", "every week", "daily", "weekly", "hourly", "schedule", "recurring", "automate", "automation"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "flow.create"}
	}
	if containsAny(lower, []string{"my flows", "list flows", "show flows", "automations", "my reminders", "scheduled"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "flow.list"}
	}
	
	// Intent-driven: user actively does something (apps)
	// Keywords: track, log, record, calculate, manage, organize
	if containsAny(lower, []string{"track", "log", "record", "calculate", "manage", "organize", "list my", "keep track"}) &&
		!containsAny(lower, []string{"remind", "alert", "notify", "send me"}) {
		return &Intent{Type: IntentCoding, Provider: ai.ProviderAnthropic, Tool: "apps.create"}
	}
	
	// Explicit app building
	if containsAny(lower, []string{"build", "create", "make", "develop", "code"}) &&
		containsAny(lower, []string{"app", "application", "website", "tool", "program", "page"}) {
		return &Intent{Type: IntentCoding, Provider: ai.ProviderAnthropic, Tool: "apps.create"}
	}
	
	// Islamic/religious - route to Reminder + Fanar
	if containsAny(lower, []string{
		"quran", "qur'an", "ayah", "ayat", "surah", "sura",
		"hadith", "hadīth", "sunnah", "sunna",
		"islam", "islamic", "muslim", "muslims",
		"allah", "prophet", "muhammad", "pbuh",
		"sharia", "shariah", "shari'a",
		"halal", "haram", "permissible", "forbidden",
		"prayer", "salah", "salat", "fasting", "sawm", "ramadan",
		"zakat", "zakah", "hajj", "pilgrimage",
		"wudu", "ablution", "ghusl",
		"imam", "mosque", "masjid",
		"dua", "du'a", "supplication",
		"jannah", "paradise", "hellfire", "jahannam",
		"angels", "jinn", "shaytan", "satan",
	}) {
		return &Intent{Type: IntentIslamic, Provider: ai.ProviderFanar, Tool: "reminder.today"}
	}
	
	// Arabic/cultural - route to Fanar
	if containsAny(lower, []string{
		"arabic", "arab", "عربي", "العربية",
		"middle east", "gulf", "qatar", "saudi", "uae", "egypt",
		"eid", "عيد", "ramadan", "رمضان",
	}) {
		return &Intent{Type: IntentArabic, Provider: ai.ProviderFanar}
	}
	
	// News search
	if containsAny(lower, []string{"news", "headlines", "article", "articles", "latest", "today's"}) &&
		containsAny(lower, []string{"search", "find", "show", "get", "what's", "whats"}) {
		return &Intent{Type: IntentNews, Provider: ai.ProviderAnthropic, Tool: "news.search"}
	}
	
	// Video
	if containsAny(lower, []string{"video", "videos", "watch", "play", "youtube"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "video.search"}
	}
	
	// Email
	if containsAny(lower, []string{"email", "mail", "send", "inbox", "message"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "mail.send"}
	}
	
	// Notes
	if containsAny(lower, []string{"note", "notes", "save", "remember", "write down"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "notes.create"}
	}
	
	// Markets/prices
	if containsAny(lower, []string{"price", "prices", "bitcoin", "btc", "eth", "crypto", "stock", "market", "gold"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "markets.get_price"}
	}
	
	// Wallet/balance
	if containsAny(lower, []string{"balance", "credits", "wallet", "how much", "funds"}) {
		return &Intent{Type: IntentTask, Provider: ai.ProviderAnthropic, Tool: "wallet.balance"}
	}
	
	// General questions - redirect to chat
	if containsAny(lower, []string{"what is", "what are", "who is", "who are", "why", "how does", "explain", "tell me about"}) &&
		!containsAny(lower, []string{"app", "video", "news", "email", "note", "price"}) {
		return &Intent{Type: IntentGeneral, Provider: ""}
	}
	
	// Unknown - will need LLM to classify
	return &Intent{Type: IntentUnknown, Provider: ai.ProviderAnthropic}
}

// containsAny checks if s contains any of the substrings
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
