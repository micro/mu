package brief

import (
	"crypto/sha256"
	"fmt"
	"mu/internal/data"
	"regexp"
	"time"
)

// ID identifies the exact edition, even after the Home summary changes.
func (e Entry) ID() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(e.Written.UTC().Format(time.RFC3339Nano)+e.Text)))
}

func Latest() (Entry, bool) {
	mu.Lock()
	defer mu.Unlock()
	if len(entries) == 0 {
		return Entry{}, false
	}
	e := entries[len(entries)-1]
	return e, e.Day == today() && e.Text != ""
}

var entryID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func Get(id string) (Entry, bool) {
	if !entryID.MatchString(id) {
		return Entry{}, false
	}
	mu.Lock()
	for _, e := range entries {
		if e.ID() == id {
			mu.Unlock()
			return e, true
		}
	}
	mu.Unlock()
	var e Entry
	err := data.LoadJSON("brief/"+id+".json", &e)
	return e, err == nil && e.ID() == id
}

// Pin retains a consulted edition beyond the rolling Home cache.
func Pin(e Entry) error { return data.SaveJSON("brief/"+e.ID()+".json", e) }
