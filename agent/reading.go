package agent

import (
	"errors"
	"strings"

	"mu/internal/saved"
	"mu/internal/thread"
)

// readingContext resolves references held by this caller's private conversation.
// It is run context, not something the user said or a fact to remember about them.
func readingContext(owner, reference string) (string, error) {
	if owner == "" || len(reference) > 256 {
		return "", errors.New("reading material not found")
	}
	kind, id, ok := strings.Cut(reference, ":")
	if !ok || id == "" {
		return "", errors.New("reading material not found")
	}
	var item *saved.Item
	var err error
	switch kind {
	case "saved":
		item, err = saved.Get(owner, id)
	case "archive":
		item, err = saved.Source(id)
	default:
		return "", errors.New("reading material not found")
	}
	if err != nil {
		return "", err
	}
	return saved.Context(item), nil
}

func conversationReading(owner, id string) string {
	ref := thread.Attachment(owner, id)
	if ref == "" {
		return ""
	}
	text, err := readingContext(owner, ref)
	if err != nil {
		return "The material previously attached to this conversation is no longer available."
	}
	return text
}
