package stream

import (
	"context"
	"time"

	"mu/internal/app"
	"mu/internal/service"
)

// Server exposes the timeline as a service, so an agent can ask what has been
// happening here rather than only a browser being able to see it.
type Server struct{}

// Item is one timeline entry, flattened for transport.
type Item struct {
	ID      string    `json:"id"`
	Service string    `json:"service" description:"Which service this came from"`
	Text    string    `json:"text"`
	URL     string    `json:"url,omitempty"`
	At      time.Time `json:"at"`
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct {
	Limit int `json:"limit,omitempty" description:"How many entries to return (default 20, max 100)"`
}

type ListResponse struct {
	Entries []Item `json:"entries" description:"Recent activity, newest first"`
}

// List returns what has happened here recently.
// @example {"limit": 10}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	// Recent filters by viewer, so a caller with no account sees only what is
	// public and never anybody else's.
	for _, e := range Recent(limit, service.AccountFrom(ctx)) {
		rsp.Entries = append(rsp.Entries, Item{
			ID: e.ID, Service: e.Service, Text: e.Text, URL: e.URL, At: e.At,
		})
	}
	if rsp.Entries == nil {
		rsp.Entries = []Item{}
	}
	return nil
}

// LoadService registers the timeline as a service. Separate from Load, which
// restores it and subscribes to the bus.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("stream", "service register failed: %v", err)
	}
}

// Card shows the last few things that happened, to whoever is looking.
func Card(v service.Viewer) string {
	items := Recent(5, v.Account)
	if len(items) == 0 {
		return `<p class="text-muted text-base">Nothing yet.</p>`
	}
	return RenderList(items)
}

var Spec = service.Spec{
	Name:        "stream",
	Handler:     new(Server),
	Description: "What has been happening here — posts, headlines, video, mail",
	Page:        "/stream",
	Icon:        "stream.svg",
	Card:        service.Personal(Card),
	Endpoints: map[string]service.Endpoint{
		// Reading only. There was a Post, and it was the console: the one way
		// anything ever reached this timeline was somebody typing into it. What
		// happened here is announced by the services it happened in.
		"List": {Aliases: []string{"stream"}, Doc: "Read what has happened here recently"},
	},
}
