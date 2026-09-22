package agent

import (
	"encoding/json"
	"mu/internal/result"
	"net/url"
	"strings"
)

// Only successful, typed tool results become durable presentation objects.
func resultItems(s Step) []result.Item {
	if !s.OK {
		return nil
	}
	var payload struct{ Content string }
	if json.Unmarshal([]byte(s.Output), &payload) != nil {
		return nil
	}
	raw := []byte(payload.Content)
	switch s.Tool {
	case "notes_add", "notes_get":
		var d struct{ Item *result.Item }
		if json.Unmarshal(raw, &d) == nil && d.Item != nil && d.Item.Kind == "note" {
			d.Item.Body = truncate(d.Item.Body, 4000)
			return []result.Item{*d.Item}
		}
		return nil
	case "docs_write", "docs_read":
		var d struct {
			Doc *struct{ ID, Title, Content string }
		}
		if json.Unmarshal(raw, &d) == nil && d.Doc != nil {
			return []result.Item{{Kind: "doc", ID: d.Doc.ID, Title: d.Doc.Title, Body: truncate(d.Doc.Content, 4000), URL: "/docs?id=" + url.QueryEscape(d.Doc.ID)}}
		}
		return nil
	case "apps_create", "apps_edit", "apps_read", "apps_build", "apps_buildstatus":
		var d struct{ Item *result.Item }
		if json.Unmarshal(raw, &d) == nil && d.Item != nil && d.Item.Kind == "app" {
			return []result.Item{*d.Item}
		}
		return nil
	case "video_search", "video_list", "video_read", "bookmarks_list", "bookmarks_get":
		var d struct {
			Results []result.Item
			Items   []result.Item
			Item    *result.Item
		}
		if json.Unmarshal(raw, &d) != nil {
			return nil
		}
		items := append(d.Results, d.Items...)
		if d.Item != nil {
			items = append(items, *d.Item)
		}
		var out []result.Item
		for _, item := range items {
			if strings.HasPrefix(s.Tool, "video_") && item.ID != "" {
				item.URL = "https://youtube.com/watch?v=" + url.QueryEscape(item.ID)
			}
			u, e := url.Parse(item.URL)
			if e != nil {
				continue
			}
			host := strings.ToLower(u.Hostname())
			id := ""
			if host == "youtube.com" || host == "www.youtube.com" {
				id = u.Query().Get("v")
			} else if host == "youtu.be" {
				id = strings.Trim(u.Path, "/")
			}
			if id != "" {
				item.Kind = "video"
				item.ID = id
			} else {
				item.Kind = "article"
			}
			if item.Title == "" {
				continue
			}
			out = append(out, item)
			if len(out) == 3 {
				break
			}
		}
		return out
	case "news_search", "news_read", "news_headlines", "news_list":
		var d struct {
			Results []struct{ Title, URL, Description string }
			Items   []struct{ Title, URL, Description string }
			Text    string
			Feed    []struct{ Title, URL, Description string }
			Item    *struct{ Title, URL, Description string }
		}
		if json.Unmarshal(raw, &d) != nil {
			return nil
		}
		if s.Tool == "news_search" && d.Text != "" {
			if json.Unmarshal([]byte(d.Text), &d) != nil {
				return nil
			}
		}
		rows := append(append(d.Results, d.Feed...), d.Items...)
		if d.Item != nil {
			rows = append(rows, *d.Item)
		}
		var out []result.Item
		for _, v := range rows {
			out = append(out, result.Item{Kind: "article", Title: v.Title, URL: v.URL, Summary: truncate(v.Description, 500)})
			if len(out) == 3 {
				break
			}
		}
		return out
	case "routes_directions":
		var d struct {
			Text, Summary string
			Shape         []result.Point
			Instructions  []string
			Estimate      bool
		}
		if json.Unmarshal(raw, &d) != nil || d.Estimate || len(d.Shape) < 2 {
			return nil
		}
		return []result.Item{{Kind: "route", Title: "Directions", Summary: d.Summary, Shape: d.Shape, Steps: d.Instructions}}
	}
	return nil
}

func mergeResults(existing, incoming []result.Item) []result.Item {
	for _, item := range incoming {
		replaced := false
		for i, old := range existing {
			if item.URL != "" && item.URL == old.URL || item.Kind == "route" && old.Kind == "route" {
				existing[i] = item
				replaced = true
				break
			}
		}
		if !replaced && len(existing) < 6 {
			existing = append(existing, item)
		}
	}
	return existing
}
