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
	// Minutes makes this a calendar entry rather than a moment: a meeting
	// occupies time, and Free needs to know how much before it can say when
	// somebody is available.
	Minutes int `json:"minutes" description:"How long it lasts in minutes (default 30)"`
	// Repeat and Prompt turn a one-off reminder into a standing instruction:
	// something that happens again, and does work when it does.
	Repeat string `json:"repeat" description:"How often it recurs: hourly, daily, weekly or monthly. Omit for once"`
	Prompt string `json:"prompt" description:"Optional instruction to run through the agent when it fires, e.g. \"brief me on today's news\". The answer is mailed to you"`
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
	e, err := CreateStanding(service.AccountFrom(ctx), req.Title, when, req.Note, req.Minutes, req.Repeat, req.Prompt)
	if err != nil {
		return err
	}
	if e.Prompt != "" {
		rsp.Result = fmt.Sprintf("Standing instruction set: %s. The answer will be mailed to you each time.", Describe(e))
		return nil
	}
	if e.Repeat != RepeatNone {
		rsp.Result = fmt.Sprintf("Scheduled: %s.", Describe(e))
		return nil
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
	owner := service.AccountFrom(ctx)

	// The id is on every line because nothing can be cancelled without one, and
	// this is the only place an agent learns it.
	var b strings.Builder
	for _, e := range Upcoming(owner) {
		fmt.Fprintf(&b, "- %s — %s", e.When.Format("Mon 2 Jan 15:04 MST"), e.Title)
		if e.Note != "" {
			fmt.Fprintf(&b, " (%s)", e.Note)
		}
		if e.Repeat != "" {
			fmt.Fprintf(&b, " [repeats %s]", e.Repeat)
		}
		fmt.Fprintf(&b, " (id: %s)", e.ID)
		b.WriteByte('\n')
	}

	// The attached calendar's entries carry no id, and say where they came
	// from. Both facts are the same fact: Mu did not schedule these and cannot
	// cancel them, so offering an id would be offering something that fails.
	now := time.Now()
	for _, x := range externalEntries(owner, now, now.Add(14*24*time.Hour)) {
		fmt.Fprintf(&b, "- %s — %s", x.Start.Format("Mon 2 Jan 15:04 MST"), x.Title)
		if x.Location != "" {
			fmt.Fprintf(&b, " (%s)", x.Location)
		}
		fmt.Fprintf(&b, " [%s]\n", ExternalName)
	}

	if b.Len() == 0 {
		rsp.Events = "No upcoming events."
	} else {
		rsp.Events = strings.TrimSpace(b.String())
	}
	rsp.Events += connectHint(owner)
	return nil
}

// DeleteRequest cancels one event.
type DeleteRequest struct {
	ID string `json:"id" description:"The event's id, as given by events_list"`
}

// DeleteResponse confirms the cancellation.
type DeleteResponse struct {
	Status string `json:"status" description:"What happened"`
}

// Delete cancels an event the caller owns.
//
// Create, Free and List were the whole surface, which made scheduling a
// one-way door: a standing instruction set to run every morning could not be
// stopped by the person paying for each run.
// @example {"id": "a1b2c3d4"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	if err := Remove(service.AccountFrom(ctx), strings.TrimSpace(req.ID)); err != nil {
		return err
	}
	rsp.Status = "Cancelled."
	return nil
}

// FreeRequest asks when the caller is free.
type FreeRequest struct {
	From     string `json:"from" description:"Start of the window to search, RFC3339, e.g. 2026-08-03T00:00:00+01:00. Defaults to now"`
	To       string `json:"to" description:"End of the window, RFC3339. Defaults to a week after from"`
	Minutes  int    `json:"minutes" description:"How long a slot you need, in minutes (default 30)"`
	DayStart int    `json:"day_start" description:"Earliest hour of the day to offer, 0-23 (default 9)"`
	DayEnd   int    `json:"day_end" description:"Latest hour of the day to offer, 0-23 (default 18)"`
}

// FreeResponse is the open slots as model-ready text.
type FreeResponse struct {
	Slots []Slot `json:"slots" description:"The open slots, in order"`
	Text  string `json:"text" description:"The same slots as readable text, grouped by day"`
}

// Free finds when the caller has nothing booked — the question a calendar is
// actually asked. Give it how long you need and it returns the stretches that
// fit, within working hours.
// @example {"minutes": 60, "from": "2026-08-03T09:00:00+01:00", "to": "2026-08-04T18:00:00+01:00"}
func (Server) Free(ctx context.Context, req *FreeRequest, rsp *FreeResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to check your calendar")
	}

	from := time.Now()
	if s := strings.TrimSpace(req.From); s != "" {
		parsed, err := parseWhen(s)
		if err != nil {
			return err
		}
		from = parsed
	}
	to := from.Add(7 * 24 * time.Hour)
	if s := strings.TrimSpace(req.To); s != "" {
		parsed, err := parseWhen(s)
		if err != nil {
			return err
		}
		to = parsed
	}

	q := FreeQuery{From: from, To: to, Minutes: req.Minutes, DayStart: req.DayStart, DayEnd: req.DayEnd}
	if req.DayEnd == 0 && req.DayStart == 0 {
		q.DayStart, q.DayEnd = 9, 18
	}
	rsp.Slots = Free(owner, q)
	want := req.Minutes
	if want <= 0 {
		want = 30
	}
	rsp.Text = RenderSlots(rsp.Slots, want)
	rsp.Text += connectHint(owner)
	return nil
}

// connectHint tells the caller when the answer was computed from one calendar
// while the person keeps another.
//
// This is the ask, placed where it is earned. Nobody is asked to hand over
// their calendar at signup; they are told, at the moment they get a thinner
// answer than they wanted, exactly what would make it a complete one. It goes
// in the text because the agent is usually the one reading this — it is how
// "you're free Thursday at 2" becomes "you're free Thursday at 2 as far as I
// can see, and I can see more if you connect your calendar".
func connectHint(owner string) string {
	if !CanConnectExternal() || HasExternal(owner) {
		return ""
	}
	return "\n\n(Only events scheduled here were checked. Connect your " +
		ExternalName + " at /events to include everything else.)"
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
	Description: "Calendar: what is scheduled, and when you are free",
	Page:        "/events",
	Scoped:      true,
	Icon:        "events.svg",
	Endpoints: map[string]service.Endpoint{
		"Create": {Doc: "Schedule a reminder or event at a given time; optionally repeating, and optionally running a prompt through the agent when it fires"},
		"Free":   {Doc: "Find when the caller has nothing booked — open slots of a given length, within working hours"},
		"List":   {Doc: "List the caller's upcoming events and reminders, each with its id"},
		"Delete": {Doc: "Cancel an event by id", Destructive: true},
	},
}
