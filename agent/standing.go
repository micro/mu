package agent

// Standing instructions: an agent doing a piece of work while nobody is
// watching.
//
// This is the most valuable thing a server that stays up can do and a stdio MCP
// process cannot — "every morning, brief me and mail it" has nowhere to live in
// a process that only exists while a client is attached. Every piece was built:
// a scheduler, an agent, an inbox, and events_create takes a prompt and a
// repeat.
//
// Nobody could find it. The only way to make one was to know that a calendar
// tool's create call has an optional prompt argument, and then to say so to the
// agent in the right words. A capability reachable only by knowing an argument
// exists is a capability nobody has.
//
// So: a list of the ones you have, on the page about what acts for you, and
// two templates that make one in a click. The templates are deliberately the
// two that prove different things — a brief proves the schedule, a watcher
// proves that an agent can remember what it saw last time.

import (
	"fmt"
	"html"
	"strings"
	"time"

	"mu/internal/app"
	"mu/service/events"
)

// StandingTemplate is a one-click standing instruction.
type StandingTemplate struct {
	Key    string
	Title  string
	Repeat string
	Hour   int // local hour to run at
	What   string
	Prompt string
}

// StandingTemplates are the starting points offered on /agents.
//
// Written as instructions to an agent, not as tool calls: the agent picks the
// tools. "Tell me what changed" is a prompt; "call markets_list then db_get" is
// a program somebody has to maintain.
var StandingTemplates = []StandingTemplate{
	{
		Key:    "brief",
		Title:  "Morning brief",
		Repeat: events.RepeatDaily,
		Hour:   7,
		What:   "Every morning at 7, what you need to know today — mailed to you.",
		Prompt: "Give me a short brief for today. Cover anything notable in the news, " +
			"how the markets opened, the weather where I am, what is on my calendar, " +
			"and anything in my inbox that looks like it needs an answer. Be brief: " +
			"if nothing is worth saying about a section, leave it out.",
	},
	{
		Key:    "watch",
		Title:  "Watcher",
		Repeat: events.RepeatHourly,
		Hour:   0, // hourly ignores the hour
		What:   "Every hour, check on something and only speak up when it changes.",
		Prompt: "Check the things I have asked you to watch. Keep what you find in a " +
			"database collection called watch, compare it with what you stored last " +
			"time, and tell me only what changed. If nothing changed, say nothing at all.",
	},
}

// standingTemplate finds a template by key.
func standingTemplate(key string) *StandingTemplate {
	for i := range StandingTemplates {
		if StandingTemplates[i].Key == key {
			return &StandingTemplates[i]
		}
	}
	return nil
}

// nextRun is when a template should first fire: the next occurrence of its
// hour, today if that is still ahead and tomorrow otherwise. Hourly starts at
// the top of the next hour.
func nextRun(t *StandingTemplate, now time.Time) time.Time {
	if t.Repeat == events.RepeatHourly {
		return now.Truncate(time.Hour).Add(time.Hour)
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), t.Hour, 0, 0, 0, now.Location())
	if !at.After(now) {
		at = at.Add(24 * time.Hour)
	}
	return at
}

// CreateStanding schedules a template as one of the owner's agents.
func CreateStanding(owner, key, agentID string) error {
	t := standingTemplate(key)
	if t == nil {
		return fmt.Errorf("no such template")
	}
	title := t.Title
	if a := AgentFor(owner, agentID); a != nil {
		title = a.Name + ": " + t.Title
	} else {
		agentID = "" // an agent that is not yours runs as the default, not as theirs
	}
	_, err := events.CreateStandingAs(owner, title, nextRun(t, time.Now()), t.What, 0,
		t.Repeat, t.Prompt, agentID)
	return err
}

// standingSection renders the standing instructions an owner has, and the
// templates for making one.
func standingSection(owner, csrf string) string {
	var mine []*events.Event
	for _, e := range events.List(owner) {
		if strings.TrimSpace(e.Prompt) != "" {
			mine = append(mine, e)
		}
	}

	var b strings.Builder
	b.WriteString(`<h3 style="font-size:15px;margin:24px 0 4px">Standing instructions</h3>`)
	b.WriteString(`<p class="text-sm" style="color:#666;margin:0 0 10px">Work an agent does on a ` +
		`schedule, whether or not you are here. The answer is mailed to you.</p>`)

	if len(mine) > 0 {
		b.WriteString(`<div style="display:flex;flex-direction:column;gap:8px;margin:0 0 12px">`)
		for _, e := range mine {
			as := "Micro"
			if a := AgentFor(owner, e.Agent); a != nil {
				as = a.Name
			}
			when := e.Repeat
			if when == "" {
				when = "once"
			}
			b.WriteString(`<div class="agent-row"><div style="flex:1;min-width:0">` +
				`<div class="agent-name">` + html.EscapeString(e.Title) + `</div>` +
				`<div class="agent-meta">` + html.EscapeString(when) + ` · as ` +
				html.EscapeString(as) + ` · next ` +
				html.EscapeString(e.When.Local().Format("2 Jan 15:04")) + `</div></div>` +
				`<form method="POST" action="/agents" style="margin:0">` +
				`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">` +
				`<input type="hidden" name="action" value="unschedule">` +
				`<input type="hidden" name="id" value="` + html.EscapeString(e.ID) + `">` +
				`<button type="submit" class="agent-remove">Stop</button></form></div>`)
		}
		b.WriteString(`</div>`)
	}

	// The agent to run it as. Only offered when there is a choice to make.
	pick := ""
	if roster := Agents(owner); len(roster) > 0 {
		var opts strings.Builder
		opts.WriteString(`<option value="">Micro (default)</option>`)
		for _, a := range roster {
			opts.WriteString(`<option value="` + html.EscapeString(a.ID) + `">` +
				html.EscapeString(a.Name) + `</option>`)
		}
		pick = `<select name="agent" class="standing-pick">` + opts.String() + `</select>`
	}

	b.WriteString(`<div class="standing-row">`)
	for _, t := range StandingTemplates {
		b.WriteString(`<form method="POST" action="/agents" class="standing-card">` +
			`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">` +
			`<input type="hidden" name="action" value="standing">` +
			`<input type="hidden" name="template" value="` + html.EscapeString(t.Key) + `">` +
			`<strong>` + html.EscapeString(t.Title) + `</strong>` +
			`<span>` + html.EscapeString(t.What) + `</span>` +
			pick +
			`<button type="submit">Schedule it</button></form>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<p class="text-sm" style="color:#888;margin:8px 0 0">Or just say it: ` +
		`&ldquo;every Friday, summarise what came in this week and mail it to me&rdquo;. ` +
		app.Link("More ideas", "/docs/usecases") + `</p>`)
	return b.String() + standingCSS
}

const standingCSS = `<style>
.standing-row{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.standing-card{display:flex;flex-direction:column;gap:6px;align-items:flex-start;
  border:1px solid var(--card-border,#e8e8e8);border-radius:8px;padding:14px 16px;margin:0;
  background:var(--card-background,#fff)}
.standing-card strong{font-size:14px;color:var(--text-primary,#111)}
.standing-card span{font-size:13px;color:#666;line-height:1.45}
.standing-pick{width:auto;padding:4px 8px;font-size:13px;font-family:inherit}
@media only screen and (max-width:600px){.standing-row{grid-template-columns:1fr}}
</style>`
