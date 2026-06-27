// Package core is mu's capability registry: the agent-era equivalent of a
// microservice service registry. Each service/app self-registers a Capability
// once (typically from its Load()), and from that single registration it shows
// up everywhere — as a visual card in the chat, the dashboard and the daily
// brief, keyed to the agent tools that should surface it.
//
// It is intentionally dependency-free so any package can import it without
// risking an import cycle.
package core

import (
	"html"
	"strings"
	"sync"
)

// Result is the unified value a capability returns: a model-ready text
// representation for the LLM and chat, an optional rich HTML card for the
// visual surface, and optional structured data. New tools/capabilities should
// return this so one handler feeds both the agent's reasoning and the feed.
type Result struct {
	Text string
	HTML string
	Data any
}

// Capability is a self-registering unit of functionality (a service or app).
type Capability struct {
	ID    string        // stable id, e.g. "markets"
	Title string        // display title, e.g. "📈 Markets"
	Card  func() string // overview card body HTML (optional)
	Tools []string      // agent tool names whose answers should attach this card
}

var (
	mu    sync.RWMutex
	caps  = map[string]*Capability{}
	order []string
)

// Register adds or replaces a capability. Safe to call from a service's Load().
func Register(c Capability) {
	if c.ID == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := caps[c.ID]; !exists {
		order = append(order, c.ID)
	}
	cp := c
	caps[c.ID] = &cp
}

// Get returns a capability by id, or nil.
func Get(id string) *Capability {
	mu.RLock()
	defer mu.RUnlock()
	return caps[id]
}

// All returns the registered capabilities in registration order.
func All() []*Capability {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]*Capability, 0, len(order))
	for _, id := range order {
		out = append(out, caps[id])
	}
	return out
}

// ForTool returns the capability whose Tools list contains tool, or nil.
func ForTool(tool string) *Capability {
	mu.RLock()
	defer mu.RUnlock()
	for _, id := range order {
		for _, t := range caps[id].Tools {
			if t == tool {
				return caps[id]
			}
		}
	}
	return nil
}

// CardHTML renders a capability's card wrapped in the standard card container,
// or "" if it has no card or the body is empty.
func CardHTML(id string) string {
	return wrapCard(Get(id))
}

// CardForTool renders the card for the capability backing the given tool.
func CardForTool(tool string) string {
	return wrapCard(ForTool(tool))
}

func wrapCard(c *Capability) string {
	if c == nil || c.Card == nil {
		return ""
	}
	body := strings.TrimSpace(c.Card())
	if body == "" {
		return ""
	}
	return `<div class="card"><h4>` + html.EscapeString(c.Title) + `</h4>` + body + `</div>`
}
