// Package alerts stores attention-worthy conditions. Detection and delivery are
// application policy; this package neither polls other services nor sends messages.
package alerts

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"mu/internal/data"
)

const MaxItems = 1000

var mu sync.Mutex
var ErrNotFound = errors.New("alert not found")

type Item struct {
	ID      string    `json:"id"`
	Source  string    `json:"source"`
	Key     string    `json:"key"`
	Title   string    `json:"title"`
	Detail  string    `json:"detail"`
	URL     string    `json:"url,omitempty"`
	State   string    `json:"state"`
	Active  bool      `json:"active"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

func key(owner string) string { return fmt.Sprintf("alerts/%x.json", sha256.Sum256([]byte(owner))) }
func read(owner string) ([]Item, error) {
	if owner == "" {
		return nil, errors.New("sign in to use alerts")
	}
	b, err := data.LoadFile(key(owner))
	if errors.Is(err, os.ErrNotExist) {
		return []Item{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []Item
	if err = json.Unmarshal(b, &items); err != nil || items == nil {
		return nil, errors.New("cannot read alerts")
	}
	return items, nil
}
func write(owner string, items []Item) error {
	b, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return data.SaveFile(key(owner), string(b))
}
func validate(i Item) error {
	if strings.TrimSpace(i.Title) == "" || i.Source == "" || i.Key == "" {
		return errors.New("source, key and title are required")
	}
	if len(i.Title) > 240 || len(i.Detail) > 4000 || len(i.URL) > 2048 || len(i.Source) > 80 || len(i.Key) > 200 {
		return errors.New("alert is too long")
	}
	if i.URL != "" {
		u, err := url.Parse(i.URL)
		if err != nil || u.User != nil || strings.ContainsAny(i.URL, "\\\r\n\t") || !((u.Scheme == "https" && u.Hostname() != "") || (u.Scheme == "" && u.Host == "" && strings.HasPrefix(i.URL, "/") && !strings.HasPrefix(i.URL, "//"))) {
			return errors.New("use a local or https action link")
		}
	}
	return nil
}

// Raise updates an existing condition without resetting acknowledgement or
// dismissal. The boolean reports a new episode, only after a durable write.
func Raise(owner string, item Item) (Item, bool, error) {
	if err := validate(item); err != nil {
		return Item{}, false, err
	}
	mu.Lock()
	defer mu.Unlock()
	items, err := read(owner)
	if err != nil {
		return Item{}, false, err
	}
	now := time.Now().UTC()
	for n, old := range items {
		if old.Source != item.Source || old.Key != item.Key {
			continue
		}
		fresh := !old.Active
		if fresh {
			old.State = "open"
			old.Created = now
		}
		old.Active = true
		old.Title = item.Title
		old.Detail = item.Detail
		old.URL = item.URL
		old.Updated = now
		items[n] = old
		err = write(owner, items)
		return old, fresh && err == nil, err
	}
	// Bound storage without evicting active conditions (including dismissed ones).
	if len(items) >= MaxItems {
		oldest := -1
		for n, i := range items {
			if !i.Active && (oldest < 0 || i.Updated.Before(items[oldest].Updated)) {
				oldest = n
			}
		}
		if oldest < 0 {
			return Item{}, false, errors.New("too many active alerts")
		}
		items = append(items[:oldest], items[oldest+1:]...)
	}
	item.ID = uuid.NewString()
	item.State = "open"
	item.Active = true
	item.Created = now
	item.Updated = now
	items = append(items, item)
	err = write(owner, items)
	return item, err == nil, err
}

// Recover is called by the condition's producer when it is no longer true.
// A user resolving a monitored condition does not manufacture recovery.
func Recover(owner, source, k string) error {
	mu.Lock()
	defer mu.Unlock()
	items, err := read(owner)
	if err != nil {
		return err
	}
	for n, i := range items {
		if i.Source == source && i.Key == k && i.Active {
			items[n].Active = false
			items[n].State = "resolved"
			items[n].Updated = time.Now().UTC()
			return write(owner, items)
		}
	}
	return nil
}
func List(owner string) ([]Item, error) {
	mu.Lock()
	defer mu.Unlock()
	items, err := read(owner)
	sort.Slice(items, func(i, j int) bool { return items[i].Updated.After(items[j].Updated) })
	return items, err
}
func Get(owner, id string) (Item, error) {
	items, err := List(owner)
	if err != nil {
		return Item{}, err
	}
	for _, i := range items {
		if i.ID == id {
			return i, nil
		}
	}
	return Item{}, ErrNotFound
}
func Update(owner, id, state string) error {
	if state != "acknowledged" && state != "resolved" && state != "dismissed" {
		return errors.New("choose acknowledged, resolved or dismissed")
	}
	mu.Lock()
	defer mu.Unlock()
	items, err := read(owner)
	if err != nil {
		return err
	}
	for n, i := range items {
		if i.ID != id {
			continue
		}
		if i.State == state {
			return nil
		}
		if i.State == "resolved" || (i.State == "dismissed" && state != "resolved") {
			return errors.New("alert is already closed")
		}
		items[n].State = state
		items[n].Updated = time.Now().UTC()
		if state == "resolved" && i.Source == "custom" {
			items[n].Active = false
		}
		return write(owner, items)
	}
	return ErrNotFound
}
func Clear(owner string) error {
	if owner == "" {
		return errors.New("owner required")
	}
	mu.Lock()
	defer mu.Unlock()
	err := data.DeleteFile(key(owner))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
