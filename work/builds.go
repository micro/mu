package work

import (
	"context"
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/event"
	"mu/internal/result"
	"mu/internal/service"
	"mu/internal/thread"
	"mu/service/apps"
	"net/url"
	"sort"
	"sync"
	"time"
)

var buildsMu sync.RWMutex
var appBuilds = map[string]map[string]apps.BuildStatusResponse{}

// Product composition owns build progress and conversation delivery. The app
// service owns jobs and publishes references; it never knows Inbox or Agent.
func loadAppBuilds() {
	go func() {
		// Restore the cache without blocking the listener or making pages join sources.
		var saved map[string]map[string]apps.BuildStatusResponse
		if data.LoadJSON("work/app-builds.json", &saved) == nil && saved != nil {
			buildsMu.Lock()
			appBuilds = saved
			buildsMu.Unlock()
		}
		go reconcileAppBuilds()
		for {
			err := event.Consume(context.Background(), "work-app-builds", []string{"apps.build.changed"}, projectAppBuild)
			app.Log("work", "app build consumer stopped: %v", err)
			time.Sleep(5 * time.Second)
		}
	}()
}

// Historical recovery is independent of live delivery and never delays pages.
func reconcileAppBuilds() {
	for _, acc := range auth.AllAccounts() {
		ctx, cancel := context.WithTimeout(service.WithAccount(context.Background(), acc.ID), 10*time.Second)
		var rsp apps.BuildsResponse
		err := service.Call(ctx, "apps", "Server.Builds", &apps.CollectionRequest{}, &rsp)
		cancel()
		if err != nil {
			app.Log("work", "restore app builds: %v", err)
			continue
		}
		for _, b := range rsp.Items {
			if err := cacheAppBuild(acc.ID, b); err != nil {
				app.Log("work", "restore app build: %v", err)
			}
		}
	}
}
func projectAppBuild(e event.Record) error {
	if acc, err := auth.GetAccount(e.Account); err != nil || acc == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var b apps.BuildStatusResponse
	if err := service.Call(service.WithAccount(ctx, e.Account), "apps", "Server.BuildStatus", &apps.BuildStatusRequest{ID: e.Resource}, &b); err != nil {
		return err
	}
	return cacheAppBuild(e.Account, b)
}
func cacheAppBuild(owner string, b apps.BuildStatusResponse) error {
	buildsMu.Lock()
	if appBuilds[owner] == nil {
		appBuilds[owner] = map[string]apps.BuildStatusResponse{}
	}
	if previous, ok := appBuilds[owner][b.ID]; ok && previous.Updated.After(b.Updated) {
		buildsMu.Unlock()
		return nil
	}
	appBuilds[owner][b.ID] = b
	err := data.SaveJSON("work/app-builds.json", appBuilds)
	buildsMu.Unlock()
	if err != nil {
		return err
	}
	if b.Thread == "" || (b.State != "complete" && b.State != "failed") || thread.Get(owner, b.Thread) == nil {
		return nil
	}
	text := "Your app could not be built: " + b.Error
	var items []result.Item
	if b.State == "complete" {
		text = "Your app is ready: " + b.URL
		if b.Item != nil {
			text = "Your app is ready: " + b.Item.Title
			items = []result.Item{*b.Item}
		}
	}
	if thread.Add(thread.Message{Account: owner, Thread: b.Thread, Role: thread.RoleAgent, Text: text, Results: items, Ref: "app-build:" + b.ID}) == "" {
		return fmt.Errorf("could not deliver app build")
	}
	return thread.Flush()
}
func buildsFor(owner string) []apps.BuildStatusResponse {
	buildsMu.RLock()
	defer buildsMu.RUnlock()
	var out []apps.BuildStatusResponse
	for _, b := range appBuilds[owner] {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}
func buildState(s string) string {
	switch s {
	case "complete":
		return "Ready"
	case "failed":
		return "Needs attention"
	case "queued":
		return "Queued"
	default:
		return "Building"
	}
}
func buildDetail(b apps.BuildStatusResponse) string {
	body := `<div class="form-actions"><a href="/work">Work</a><a href="/home/apps">My apps</a></div><h2>Build an app</h2><p>` + html.EscapeString(b.Prompt) + `</p><p>` + buildState(b.State) + ` · Updated ` + app.TimeAgo(b.Updated) + `</p>`
	if b.State == "complete" && b.Item != nil {
		body += `<p>Your app is saved. Open it to use it, or return to the conversation to request changes.</p>` + app.Results([]result.Item{*b.Item})
	}
	if b.Error != "" {
		body += `<p>` + html.EscapeString(b.Error) + `</p><p>Review the request in your conversation before asking Micro to try again.</p>`
	}
	if b.State != "complete" && b.State != "failed" {
		body += `<p>You can leave this page. The result will return to your conversation.</p><a href="/work?build=` + url.QueryEscape(b.ID) + `">Refresh status</a>`
	}
	if b.Thread != "" {
		body += `<p><a href="/?session=` + url.QueryEscape(b.Thread) + `">Continue conversation</a></p>`
	}
	return body
}
