package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/service"
)

// Server is the go-micro service handler for events. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools — so "remind
// me to X at Y" reaches Create, and "what's coming up" reaches List.
type Server struct{}

// CreateRequest schedules a reminder/event.
type CreateRequest struct {
	Title string `json:"title" description:"What to be reminded about, e.g. 'Call the dentist'"`
	// When is an RFC3339 timestamp with a timezone offset. Resolve relative
	// phrases ("tomorrow at 3pm") against the current date before calling.
	When string `json:"when" description:"When to fire, RFC3339 with timezone offset, e.g. 2026-07-22T15:00:00+01:00"`
	Note string `json:"note" description:"Optional extra detail"`
}

// CreateResponse confirms the scheduled event.
type CreateResponse struct {
	Result string `json:"result" description:"Confirmation of the scheduled event"`
}

// Create schedules a reminder or event to fire at a given time. Use this
// whenever the user asks to be reminded of something or to schedule an event.
// @example {"title": "Call the dentist", "when": "2026-07-22T15:00:00+01:00"}
func (Server) Create(ctx context.Context, req *CreateRequest, rsp *CreateResponse) error {
	when, err := parseWhen(req.When)
	if err != nil {
		return err
	}
	e, err := Create(service.AccountFrom(ctx), req.Title, when, req.Note)
	if err != nil {
		return err
	}
	rsp.Result = fmt.Sprintf("Scheduled %q for %s. Add to Google Calendar: %s",
		e.Title, e.When.Format("Mon 2 Jan 2006 15:04 MST"), GoogleCalendarURL(e.Title, e.When, e.Note))
	return nil
}

// ListRequest asks for the caller's upcoming events.
type ListRequest struct{}

// ListResponse is the caller's upcoming events as model-ready text.
type ListResponse struct {
	Events string `json:"events" description:"The caller's upcoming events, soonest first"`
}

// List returns the caller's upcoming (not-yet-fired) events.
// @example {}
func (Server) List(ctx context.Context, _ *ListRequest, rsp *ListResponse) error {
	var b strings.Builder
	for _, e := range Upcoming(service.AccountFrom(ctx)) {
		fmt.Fprintf(&b, "- %s — %s", e.When.Format("Mon 2 Jan 15:04 MST"), e.Title)
		if e.Note != "" {
			fmt.Fprintf(&b, " (%s)", e.Note)
		}
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		rsp.Events = "No upcoming events."
	} else {
		rsp.Events = strings.TrimSpace(b.String())
	}
	return nil
}

// parseWhen accepts RFC3339 (preferred, carries a timezone) and a few common
// fallbacks. Timestamps without an offset are interpreted as UTC.
func parseWhen(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("a time is required (RFC3339, e.g. 2026-07-22T15:00:00+01:00)")
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse time %q; use RFC3339 like 2026-07-22T15:00:00+01:00", s)
}

var Spec = service.Spec{
	Name:        "events",
	Handler:     new(Server),
	Description: "Scheduled reminders and calendar invites",
	Page:        "/events",
	Scoped:      true,
	Icon:        "events.svg",
	Endpoints: map[string]service.Endpoint{
		"Create": {Doc: "Schedule a reminder or event to fire at a given time"},
		"List":   {Doc: "List the caller's upcoming events and reminders"},
	},
}
