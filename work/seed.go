package work

import (
	"time"

	"github.com/google/uuid"
)

// seedPosts creates initial work posts
func seedPosts() {
	seeds := []struct {
		kind        string
		title       string
		description string
		link        string
		cost        int
		tags        string
	}{
		// Show posts — examples of what sharing looks like
		{
			kind:        KindShow,
			title:       "Mu — Apps Without Ads",
			description: "Built a platform that brings together the daily tools people use — news, search, chat, video, email, markets — in one place with zero ads and zero tracking. Single Go binary, self-hostable, open source. AI agents can access everything via MCP with x402 crypto payments.",
			link:        "https://mu.xyz",
			tags:        "platform,go,open-source",
		},
		{
			kind:        KindShow,
			title:       "x402 — HTTP Payments Protocol",
			description: "The x402 protocol adds native payment semantics to HTTP. When a resource requires payment, the server returns 402 with payment requirements. The client pays on-chain and retries. No accounts, no API keys. Designed for AI agents paying for API access.",
			link:        "https://x402.org",
			tags:        "protocol,crypto,payments",
		},
	}

	now := time.Now()

	mutex.Lock()
	for i, s := range seeds {
		post := &Post{
			ID:          uuid.New().String(),
			Kind:        s.kind,
			Title:       s.title,
			Description: s.description,
			Link:        s.link,
			Cost:        s.cost,
			AuthorID:    "micro",
			Author:      "Micro",
			Tags:        s.tags,
			Feedback:    []Comment{},
			CreatedAt:   now.Add(-time.Duration(i) * time.Minute),
		}
		if s.kind == KindTask {
			post.Status = StatusOpen
		}
		posts[post.ID] = post
	}
	save()
	mutex.Unlock()
}
