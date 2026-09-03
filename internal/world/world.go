// Package world is what this instance knows is true, and what has just
// changed.
//
// # Why it exists
//
// An agent's context was assembled by reaching into service packages at the
// moment somebody asked a question — see agent.nowContext, which this is the
// other half of. That works and it is the wrong shape: every reader has to know
// which services to ask, a service is on the answer path whether it wants to be
// or not, and nothing but the agent can use any of it.
//
// The layering underneath is already there. Services publish what they know on
// the read plane (internal/snapshot) the same way they publish their cards, and
// they announce what happens on the bus (internal/event). This subscribes to
// both and keeps the result: state per source, and a bounded record of what has
// happened lately.
//
// # State and delta
//
// Two questions, and the second is the one nothing could answer. "What is true
// now" is a snapshot — the headlines, the prices — and every service that has
// one publishes it. "What has changed" is a fold over the stream, and it is
// what makes a view a view rather than a cache: an agent that knows only the
// current state cannot tell you that something happened, only what is.
//
// # No model in here
//
// Deliberately. A model cannot be called per event — there are thousands — and
// a view that summarises as it goes is a view that costs money to hold. This is
// the deterministic layer: it can be dropped and rebuilt from the sources at
// any time, which is what makes anything built on top of it safe. A model
// reading this and writing down what it means is the layer above, and its
// output must always be recoverable from this one; the moment it is not, it has
// stopped being a view and become a second source of truth that nothing can
// audit.
//
// # Nothing personal is in here
//
// The rule service/archive states for the index and event.Announce states for
// the timeline: an Activity carrying an account is somebody's own — a message
// that arrived, an image they generated — and only that account ever sees it.
// This holds one structure read by every agent on the instance, so anything
// with an account on it is dropped at the door rather than filtered on the way
// out. Personal context reaches an agent by its own route, scoped to whoever is
// asking.
package world

import (
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/event"
	"mu/internal/service"
	"mu/internal/snapshot"
)

// A Change is one thing that happened, as the instance heard about it.
type Change struct {
	At      time.Time
	Service string // the service it came from — "news", "video"
	Text    string // what happened, in that service's own words
	URL     string // where it is, when there is somewhere
}

// remembered is how many changes are kept.
//
// Bounded because this is memory shared by every reader and an unbounded one is
// a leak with a slow fuse. Fifty is more than any prompt would carry and enough
// for a model pass over "what has been happening" to have something to work
// with.
const remembered = 50

// forgotten is how old a change may be before it stops being news.
//
// A day. What changed last week is the archive's question — it is indexed,
// searchable, and not what "lately" means to somebody asking now.
const forgotten = 24 * time.Hour

var (
	mu      sync.RWMutex
	changes []Change // oldest first
	started bool
)

// Watch starts listening. Called once, from wherever the instance is wired up.
//
// Idempotent, because a second subscription is a second handler doing identical
// work and every change recorded twice.
func Watch() {
	mu.Lock()
	if started {
		mu.Unlock()
		return
	}
	started = true
	mu.Unlock()

	// Subscribed here and consumed in the goroutine, not both there: after
	// Watch returns the caller is listening. Subscribing inside the goroutine
	// means anything announced in the next microsecond is announced to nobody,
	// which in a test is a flake and at boot is the first minute of the world
	// going unrecorded.
	sub := event.Subscribe(event.Activity)
	go func() {
		for e := range sub.Chan {
			record(e)
		}
	}()
}

// record keeps one announcement, if it is the world's and not somebody's.
func record(e event.Event) {
	if account, _ := e.Data["account"].(string); strings.TrimSpace(account) != "" {
		// Somebody's own. See the package comment: this structure is read by
		// every agent, so a personal fact is dropped at the door.
		return
	}
	svc, _ := e.Data["service"].(string)
	text, _ := e.Data["text"].(string)
	url, _ := e.Data["url"].(string)
	if strings.TrimSpace(svc) == "" || strings.TrimSpace(text) == "" {
		return
	}

	c := Change{At: time.Now().UTC(), Service: svc,
		Text: strings.TrimSpace(text), URL: strings.TrimSpace(url)}

	mu.Lock()
	defer mu.Unlock()
	// The same thing announced twice is one thing.
	//
	// An announcement carries no id, so what makes two of them the same is
	// what they say: the service and the words. news announces its top headline
	// once an hour and only when it changes, which is right until the process
	// restarts and announces the same one again — and a restart is exactly what
	// a deploy is. Without this, "what has changed" fills up with the same
	// change.
	//
	// Compared against what is still held rather than against everything ever
	// seen, which is the same window a reader gets: an event repeating tomorrow
	// is news again, and it should be.
	for _, have := range changes {
		if have.Service == c.Service && have.Text == c.Text {
			return
		}
	}
	changes = append(changes, c)
	trimLocked()
}

// trimLocked drops what is too old and then what is too much.
//
// Age first: a quiet instance should forget yesterday even when it has not
// filled the buffer, and a busy one should keep the newest fifty of today
// rather than fifty spread over a week.
func trimLocked() {
	cut := time.Now().UTC().Add(-forgotten)
	keep := changes[:0]
	for _, c := range changes {
		if c.At.After(cut) {
			keep = append(keep, c)
		}
	}
	changes = keep
	if len(changes) > remembered {
		changes = changes[len(changes)-remembered:]
	}
}

// Lately is what has happened, newest first, from the named services.
//
// No services named is every service, which is the whole world. A caller that
// passes a scope gets only what it could have asked about itself — see
// agent.nowContext, which passes the agent's own tools.
func Lately(services ...string) []Change {
	want := map[string]bool{}
	for _, s := range services {
		want[strings.ToLower(strings.TrimSpace(s))] = true
	}

	mu.RLock()
	defer mu.RUnlock()
	trimmedCut := time.Now().UTC().Add(-forgotten)
	out := make([]Change, 0, len(changes))
	for _, c := range changes {
		if c.At.Before(trimmedCut) {
			continue
		}
		if len(want) > 0 && !want[strings.ToLower(c.Service)] {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// State is what each named service says is true now, keyed by service.
//
// Read from the plane, so this is map reads and no service is asked anything.
// A service that has published nothing is absent rather than empty: there is a
// difference between a service with nothing to say and one that has not spoken
// yet, and only the second is worth waiting for.
func State(services ...string) map[string]string {
	want := map[string]bool{}
	for _, s := range services {
		want[strings.ToLower(strings.TrimSpace(s))] = true
	}
	out := map[string]string{}
	for _, spec := range service.Nows() {
		if len(want) > 0 && !want[spec.Name] {
			continue
		}
		if v := strings.TrimSpace(snapshot.Value(spec.Name, "now")); v != "" {
			out[spec.Name] = v
		}
	}
	return out
}

// Forget empties the record. For tests, and for an operator who has just
// changed what is published and wants the old facts gone.
func Forget() {
	mu.Lock()
	changes = nil
	mu.Unlock()
}
