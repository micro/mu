// Package stream is the timeline of what this instance has been doing: a post
// published, a video found, a headline broken, an image generated, a message
// that arrived. Your stream.
//
// Nobody posts to it. That is the whole difference from what it was, which was
// a console — a box you typed in and the agent answered, on the home page,
// beside an /agents page and an /inbox that already do that better. Five of its
// six event types had no publisher at all, so the only thing that ever reached
// the timeline was the chat.
//
// It owns nothing. Every entry is a fact some service announced on the bus
// (event.EventActivity) and still owns; this holds a bounded tail of them and
// renders it. Delete the package and the news is still news, the posts are
// still posts — you lose the one place they were shown together. That is the
// same test service/recall passes over internal/thread.
//
// It is not social, which is what people write, and not the inbox, which is
// what arrived for you.
package stream

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/event"
)

// Entry is one thing that happened.
//
// Account is the whole privacy model: empty means anybody may see it, set
// means only that account ever does. The old timeline had no such field and
// three call sites published somebody's private content to a page served
// without a session — a fired reminder's title, a standing instruction's
// title, and an inbound sender and subject. Whoever announces a fact decides
// which of the two it is, at the point where they still know.
type Entry struct {
	ID      string    `json:"id"`
	Service string    `json:"service"`
	Text    string    `json:"text"`
	URL     string    `json:"url,omitempty"`
	Account string    `json:"account,omitempty"`
	At      time.Time `json:"at"`
}

// MaxEntries is how many are kept, in memory and on disk.
const MaxEntries = 500

// MaxTextLength caps one entry's text.
const MaxTextLength = 512

var (
	mu      sync.RWMutex
	entries []*Entry // newest first
)

// Load restores the timeline and subscribes to the bus.
//
// The stored file is the same stream.json the console wrote, and its entries
// do not fit this shape — they had a type and a content, not a service and a
// text — so they arrive with nothing in the fields that matter and are dropped
// by valid. Saving straight after is what actually removes them, which is
// wanted: some of them were people's mail.
//
// The same save takes out the mail rows this package wrote itself. See
// theirsAlone.
func Load() {
	var loaded []*Entry
	if b, err := data.LoadFile("stream.json"); err == nil {
		_ = json.Unmarshal(b, &loaded)
	}

	mu.Lock()
	for _, e := range loaded {
		if valid(e) && !theirsAlone(e) {
			entries = append(entries, e)
		}
	}
	save()
	n := len(entries)
	mu.Unlock()

	sub := event.Subscribe(event.EventActivity)
	go func() {
		for e := range sub.Chan {
			add(fromEvent(e.Data))
		}
	}()

	app.Log("stream", "Loaded %d entries", n)
}

// theirsAlone is an entry that was never this page's to show.
//
// Mail was a row here: sender, subject, and a link to /inbox. Three things were
// wrong with it and they compound.
//
// It is a duplicate of somewhere better. Mail has unread state, a thread and a
// reply, and a timeline row carries none of them — the URL was literally
// "/inbox", so the row's whole content was a pointer at the page that already
// holds it, rendered worse.
//
// It is a notification, and this instance deliberately has none. "Something
// arrived, go and look" is what a notification says; correspondence is read
// when you come back, which is the reason a reply to it is considered rather
// than reflexive.
//
// And it is the one private thing on a public timeline. Everything else here
// is a headline, a post, a video — ownerless by nature. Account exists because
// mail did, and the note on that field records what it cost the first time.
//
// Dropped on load rather than left to age out, because 500 entries is however
// many months on a quiet instance, and no operator should have to be told to
// delete a file.
func theirsAlone(e *Entry) bool { return e.Service == "mail" }

func valid(e *Entry) bool {
	return e != nil && e.Service != "" && e.Text != "" && !e.At.IsZero()
}

// fromEvent flattens an announced fact into an entry.
func fromEvent(data map[string]any) *Entry {
	str := func(k string) string {
		s, _ := data[k].(string)
		return s
	}
	return &Entry{
		Service: str("service"),
		Text:    str("text"),
		URL:     str("url"),
		Account: str("account"),
	}
}

// add appends an entry, newest first, and trims to MaxEntries.
//
// A repeat is dropped. news and video each announce the top of their feed and
// remember what they last said in a package variable, which is empty again
// after a restart — so a redeploy re-announced the current headline and the
// current video, and five restarts in an afternoon put five identical rows on
// the timeline. Guarding it here rather than in each announcer is the same
// choice as everywhere else in this package: whoever announces a fact should
// not have to know how the timeline is kept.
func add(e *Entry) {
	if e == nil || e.Service == "" || e.Text == "" {
		return
	}
	if repeat(e) {
		return
	}
	if len(e.Text) > MaxTextLength {
		e.Text = e.Text[:MaxTextLength-1] + "…"
	}
	if e.ID == "" {
		e.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}

	mu.Lock()
	entries = append([]*Entry{e}, entries...)
	if len(entries) > MaxEntries {
		entries = entries[:MaxEntries]
	}
	save()
	mu.Unlock()
}

// repeat reports whether the timeline already carries this entry.
//
// Keyed on the service and the link, because the link is what identifies a
// story; on the text when there is no link, which is what a personal entry has.
// The whole tail is searched rather than the last few: a headline that stays
// top of the feed all day would otherwise come back every time something else
// pushed it down the list.
func repeat(e *Entry) bool {
	key := e.URL
	if key == "" {
		key = e.Text
	}
	mu.RLock()
	defer mu.RUnlock()
	for _, x := range entries {
		if x.Service != e.Service || x.Account != e.Account {
			continue
		}
		if x.URL == key || (x.URL == "" && x.Text == key) {
			return true
		}
	}
	return false
}

// save writes the timeline. Callers hold mu.
func save() {
	data.SaveJSON("stream.json", entries)
}

// visible reports whether viewer may see this entry. An entry with no account
// is public; one with an account is that account's alone.
func visible(e *Entry, viewer string) bool {
	return e.Account == "" || e.Account == viewer
}

// Recent returns the newest entries visible to viewer, newest first.
func Recent(max int, viewer string) []*Entry {
	mu.RLock()
	defer mu.RUnlock()

	var result []*Entry
	for _, e := range entries {
		if !visible(e, viewer) {
			continue
		}
		result = append(result, e)
		if len(result) >= max {
			break
		}
	}
	return result
}

// Since returns entries newer than the given time, visible to viewer.
func Since(since time.Time, viewer string) []*Entry {
	mu.RLock()
	defer mu.RUnlock()

	var result []*Entry
	for _, e := range entries {
		if !e.At.After(since) {
			break // newest-first, so once we pass since we are done
		}
		if visible(e, viewer) {
			result = append(result, e)
		}
	}
	return result
}

// CountSince returns how many entries are newer than since, for viewer.
func CountSince(since time.Time, viewer string) int {
	return len(Since(since, viewer))
}

// DeleteByAccount removes the entries that were somebody's own, when their
// account goes. Public entries are unaffected: a post that was published still
// was, and the blog service is what decides whether it survives.
func DeleteByAccount(id string) {
	if id == "" {
		return
	}
	mu.Lock()
	var kept []*Entry
	for _, e := range entries {
		if e.Account != id {
			kept = append(kept, e)
		}
	}
	entries = kept
	save()
	mu.Unlock()
}

// Clear wipes the timeline. Admin use only.
func Clear() {
	mu.Lock()
	entries = nil
	save()
	mu.Unlock()
}
