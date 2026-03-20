package social

import (
	"crypto/md5"
	"fmt"
	"time"

	"mu/internal/app"
)

// maxDailyNotes is the maximum number of community notes seeded per day.
// Keeps the feed focused on the most noteworthy fact-checks rather than
// becoming a mirror of the news feed.
const maxDailyNotes = 5

// NewsArticle is a lightweight news item for fact-checking.
// Populated via callback wired in main.go to avoid importing the news package.
type NewsArticle struct {
	Title       string
	Description string
	URL         string
	Category    string // e.g. "Tech", "Finance", "World"
}

// GetRecentNews returns recent news articles (one per topic) for fact-checking.
// Wired in main.go: social.GetRecentNews = func() []social.NewsArticle { ... }
var GetRecentNews func() []NewsArticle

// seedNewsNotes picks recent news articles and fact-checks them.
// Only posts when something is genuinely misleading or missing important
// context — accurate articles don't need a note. Capped at maxDailyNotes
// per day to keep the feed focused.
func seedNewsNotes() {
	if GetRecentNews == nil {
		return
	}

	// Check how many notes we've already posted today
	if countTodayNotes() >= maxDailyNotes {
		return
	}

	articles := GetRecentNews()
	if len(articles) == 0 {
		return
	}

	for _, article := range articles {
		if countTodayNotes() >= maxDailyNotes {
			break
		}
		seedArticleNote(article)
	}
}

func seedArticleNote(article NewsArticle) {
	// Use URL hash as seed ID to avoid duplicates
	hash := fmt.Sprintf("%x", md5.Sum([]byte(article.URL)))
	seedID := "note-" + hash[:12]

	if threadExists(seedID) {
		return
	}

	// Fact-check the article's claims
	claim := article.Title
	if article.Description != "" {
		claim += "\n\n" + article.Description
	}

	note := runFactCheck(claim)
	if note == nil {
		return
	}

	// Only post when there's something worth flagging.
	// Accurate and no-claims articles don't need a community note —
	// the news feed already shows them. The value is catching what's
	// wrong or incomplete.
	if note.Status != "misleading" && note.Status != "missing_context" {
		return
	}

	topic := mapCategoryToTopic(article.Category)

	thread := &Thread{
		ID:        seedID,
		Title:     article.Title,
		Link:      article.URL,
		Content:   article.Description,
		Topic:     topic,
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
		Note:      note,
	}

	AddSeededThread(thread)
	app.Log("social", "Seeded community note [%s]: %s — %s", note.Status, article.Title, article.Category)
}

// countTodayNotes counts how many community note threads were seeded today.
func countTodayNotes() int {
	today := todayKey()
	mutex.RLock()
	defer mutex.RUnlock()

	count := 0
	for _, t := range threads {
		if t.AuthorID != app.SystemUserID {
			continue
		}
		if t.Note == nil {
			continue
		}
		if t.CreatedAt.Format("2006-01-02") != today {
			continue
		}
		count++
	}
	return count
}

// mapCategoryToTopic maps a news feed category to a valid social topic.
func mapCategoryToTopic(category string) string {
	switch category {
	case "Crypto":
		return "Crypto"
	case "Dev":
		return "Dev"
	case "Finance":
		return "Finance"
	case "Islam":
		return "Islam"
	case "Politics":
		return "Politics"
	case "Tech":
		return "Tech"
	case "UK":
		return "UK"
	case "World":
		return "World"
	default:
		return "World"
	}
}
