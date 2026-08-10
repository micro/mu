package stream

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/billing"
	"mu/internal/app"
	"mu/internal/service"
)

// Server exposes the console timeline as a service, so the agent and apps can
// read what has been happening and post into it rather than only the web
// handler being able to.
type Server struct{}

// Entry is one timeline event, flattened for transport.
type Entry struct {
	ID        string    `json:"id"`
	Type      string    `json:"type" description:"user, agent, system, market, news or reminder"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct {
	Limit int `json:"limit,omitempty" description:"How many events to return (default 20, max 100)"`
}

type ListResponse struct {
	Events []Entry `json:"events" description:"Recent console events, newest first"`
}

// List returns recent events from the console timeline.
// @example {"limit": 10}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	// Recent filters by viewer, so a guest sees only what is public to them.
	for _, e := range Recent(limit, service.AccountFrom(ctx)) {
		rsp.Events = append(rsp.Events, Entry{
			ID: e.ID, Type: e.Type, Author: e.Author,
			Content: e.Content, CreatedAt: e.CreatedAt,
		})
	}
	if rsp.Events == nil {
		rsp.Events = []Entry{}
	}
	return nil
}

// ── Post ────────────────────────────────────────────────────────

type PostRequest struct {
	Content string `json:"content" description:"What to post to the console"`
}

type PostResponse struct {
	ID string `json:"id" description:"The new event's id"`
}

// Post adds an entry to the caller's console timeline.
// @example {"content": "Deployed the new build"}
func (Server) Post(ctx context.Context, req *PostRequest, rsp *PostResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to post to the console")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return fmt.Errorf("content is required")
	}
	e := PostUser(who, content)
	if e == nil {
		return fmt.Errorf("could not post")
	}
	rsp.ID = e.ID
	return nil
}

// LoadService registers the console as a service. Separate from Load, which
// already restores the timeline and starts its background work.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("stream", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "stream",
	Handler:     new(Server),
	Description: "The console: this instance's own event timeline",
	Page:        "/stream",
	Icon:        "stream.svg",
	Endpoints: map[string]service.Endpoint{
		"List": {Doc: "Read recent events from the console timeline"},
		// Account, because posting needs a caller while reading does not. The
		// hand-written stream_post in internal/api/mcp.go currently overrides
		// this one and forwards the session; declaring it here means the derived
		// tool works on its own if that registration ever goes.
		"Post": {Doc: "Post an entry to the console timeline", Cost: billing.OpStreamPost, Account: true},
	},
}
