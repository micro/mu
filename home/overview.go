package home

import (
	"html"
	"mu/internal/notes"
	"mu/service/apps"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/tasks"
	"net/url"
	"sort"
	"strings"
	"time"
)

type homeItem struct {
	title, kind, href string
	updated           time.Time
}

// recentItems reads the owner's records without running tools or opening a workspace.
func recentItems(owner string) []homeItem {
	if owner == "" {
		return nil
	}
	var items []homeItem
	for _, t := range tasks.List(owner, "") {
		if t.Assignee == tasks.Agent && !t.Archived {
			items = append(items, homeItem{t.Title, "Work · " + t.Status, "/work?id=" + url.QueryEscape(t.ID), t.Updated})
		}
	}
	for _, n := range notes.All(owner) {
		items = append(items, homeItem{n.Title, "Note", "/notes?id=" + url.QueryEscape(n.ID), n.UpdatedAt})
	}
	for _, d := range docs.All(owner, "", 500) {
		items = append(items, homeItem{d.Title, "Document", "/docs?id=" + url.QueryEscape(d.ID), d.Updated})
	}
	for _, f := range files.List(owner) {
		items = append(items, homeItem{f.Name, "File", "/files/" + url.PathEscape(f.ID), f.Created})
	}
	for _, a := range apps.OwnedBy(owner) {
		items = append(items, homeItem{a.Name, "App", "/apps/" + url.PathEscape(a.Slug), a.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].updated.Equal(items[j].updated) {
			return items[i].href < items[j].href
		}
		return items[i].updated.After(items[j].updated)
	})
	if len(items) > 6 {
		items = items[:6]
	}
	return items
}

func homeIndex(owner, zone string) string {
	if owner == "" {
		return ""
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc = time.UTC
	}
	var b strings.Builder
	b.WriteString(`<section class="home-index" aria-label="Your work"><nav class="view-switch" aria-label="Your items"><a href="/work">Work</a><a href="/events">Events</a><a href="/notes">Notes</a><a href="/files">Files</a><a href="/docs">Documents</a><a href="/apps">Apps</a></nav>`)
	upcoming := events.Upcoming(owner)
	sort.Slice(upcoming, func(i, j int) bool {
		if upcoming[i].When.Equal(upcoming[j].When) {
			return upcoming[i].ID < upcoming[j].ID
		}
		return upcoming[i].When.Before(upcoming[j].When)
	})
	if len(upcoming) > 3 {
		upcoming = upcoming[:3]
	}
	if len(upcoming) > 0 {
		b.WriteString(`<h2>Upcoming</h2><div class="collection-list">`)
		for _, e := range upcoming {
			writeHomeItem(&b, homeItem{e.Title, e.When.In(loc).Format("Mon 2 Jan · 15:04 MST"), "/events?id=" + url.QueryEscape(e.ID), e.When}, loc, false)
		}
		b.WriteString(`</div>`)
	}
	if items := recentItems(owner); len(items) > 0 {
		b.WriteString(`<h2>Recent</h2><div class="collection-list">`)
		for _, item := range items {
			writeHomeItem(&b, item, loc, true)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

func writeHomeItem(b *strings.Builder, item homeItem, loc *time.Location, date bool) {
	title := item.title
	if strings.TrimSpace(title) == "" {
		title = "Untitled"
	}
	b.WriteString(`<a class="collection-item" href="` + html.EscapeString(item.href) + `"><span class="collection-title">` + html.EscapeString(title) + `</span><span class="collection-preview">` + html.EscapeString(item.kind) + `</span>`)
	if date && !item.updated.IsZero() {
		b.WriteString(`<time class="collection-when" datetime="` + item.updated.Format(time.RFC3339) + `">` + html.EscapeString(item.updated.In(loc).Format("2 Jan")) + `</time>`)
	}
	b.WriteString(`</a>`)
}
