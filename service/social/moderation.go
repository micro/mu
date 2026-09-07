package social

import (
	"mu/internal/data"
	"mu/internal/flag"
)

type moderationStore struct{}

func (moderationStore) RefreshCache() {
	mutex.Lock()
	updateCacheLocked()
	mutex.Unlock()
}

func (moderationStore) Get(id string) interface{} {
	mutex.RLock()
	defer mutex.RUnlock()
	m := getMessage(id)
	if m == nil {
		return nil
	}
	return flag.PostContent{ID: m.ID, Title: "Social thread", Content: m.Content, Author: m.Author, AuthorID: m.AuthorID, CreatedAt: m.PostedAt}
}

func (moderationStore) Delete(id string) error {
	mutex.Lock()
	kept := make([]*Message, 0, len(messages))
	var removed []string
	for _, m := range messages {
		if m.ID == id || m.ReplyTo == id {
			removed = append(removed, m.ID)
			continue
		}
		kept = append(kept, m)
	}
	messages = kept
	updateCacheLocked()
	mutex.Unlock()
	if err := save(); err != nil {
		return err
	}
	for _, id := range removed {
		if err := data.Unindex("social_" + id); err != nil {
			return err
		}
	}
	return nil
}
