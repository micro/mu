package social

import (
	"crypto/md5"
	"fmt"
	"time"

	"mu/internal/app"
)

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

// seedNewsNotes picks recent news articles and fact-checks them,
// posting the results as community note threads. This runs hourly
// and creates one thread per article, with the fact-check note attached.
func seedNewsNotes() {
	if GetRecentNews == nil {
		return
	}

	articles := GetRecentNews()
	if len(articles) == 0 {
		return
	}

	for _, article := range articles {
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

	// Skip if no verifiable claims — not worth posting
	if note.Status == "none" {
		return
	}

	// Map news category to a social topic
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
