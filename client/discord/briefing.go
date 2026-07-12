package discord

import (
	"fmt"

	"mu/internal/ai"
)

// SummariseEmail generates a short summary of an email using the background model.
func SummariseEmail(from, subject, body string) string {
	if len(body) > 2000 {
		body = body[:2000]
	}
	prompt := fmt.Sprintf("From: %s\nSubject: %s\n\n%s", from, subject, body)
	result, err := ai.Ask(&ai.Prompt{
		System:   "Summarise this email in one sentence. Be specific — include the key information or action required.",
		Question: prompt,
		Model:    ai.BackgroundModel(),
		Priority: ai.PriorityLow,
		Caller:   "email-summary",
	})
	if err != nil {
		return subject
	}
	return result
}
