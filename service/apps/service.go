package apps

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for apps.
type Server struct{}

// BuildRequest describes an app to generate. The owner is the authenticated
// caller, taken from the call context — never a model-supplied author.
type BuildRequest struct {
	Prompt string `json:"prompt" required:"true" description:"Description of the app to build, e.g. 'an expense tracker', 'a packing checklist', 'a water intake counter'"`
}

// BuildResponse is the saved app's identity and URLs.
type BuildResponse struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	URL  string `json:"url"`
	Run  string `json:"run"`
}

// Build generates a small app (tracker, checklist or counter) from a natural
// language description, saves it, and returns its details with URLs.
// @example {"prompt": "an expense tracker"}
func (Server) Build(ctx context.Context, req *BuildRequest, rsp *BuildResponse) error {
	account := service.AccountFrom(ctx)
	if strings.TrimSpace(account) == "" {
		return fmt.Errorf("authentication required to build an app")
	}
	a, err := BuildApp(req.Prompt, account, AuthorNameFor(account))
	if err != nil {
		return err
	}
	rsp.Name = a.Name
	rsp.Slug = a.Slug
	rsp.URL = "/apps/" + a.Slug
	rsp.Run = "/apps/" + a.Slug
	return nil
}

// AppSearchRequest searches the apps directory.
type AppSearchRequest struct {
	Query string `json:"query" description:"Search query — name, description or tag"`
	Tag   string `json:"tag" description:"Filter to apps carrying this tag"`
}

// AppSearchResponse is a model-ready list of matching apps.
type AppSearchResponse struct {
	Text string `json:"text" description:"Matching apps: name, slug and description"`
}

// Search searches the apps directory for small, useful tools.
// @example {"query": "tracker"}
func (Server) Search(_ context.Context, req *AppSearchRequest, rsp *AppSearchResponse) error {
	results := SearchApps(req.Query)

	// A tag narrows the result rather than being a second kind of search: an
	// empty query with a tag lists everything carrying it, which is what
	// "show me the trackers" means.
	if tag := strings.ToLower(strings.TrimSpace(req.Tag)); tag != "" {
		var kept []*App
		for _, a := range results {
			for _, t := range strings.Split(a.Tags, ",") {
				if strings.ToLower(strings.TrimSpace(t)) == tag {
					kept = append(kept, a)
					break
				}
			}
		}
		results = kept
	}

	if len(results) == 0 {
		what := req.Query
		if what == "" {
			what = req.Tag
		}
		rsp.Text = "No apps found for \"" + what + "\"."
		return nil
	}
	var b strings.Builder
	for _, a := range results {
		b.WriteString("- " + a.Name + " (" + a.Slug + "): " + a.Description + "\n")
	}
	rsp.Text = b.String()
	return nil
}

// AppReadRequest reads one app by slug.
type AppReadRequest struct {
	Slug string `json:"slug" required:"true" description:"The app's URL slug, e.g. pomodoro-timer"`
}

// AppReadResponse is a model-ready description of an app.
type AppReadResponse struct {
	Text string `json:"text"`
}

// Read returns the details of a specific app by its slug.
// @example {"slug": "expense-tracker"}
func (Server) Read(_ context.Context, req *AppReadRequest, rsp *AppReadResponse) error {
	a := GetApp(req.Slug)
	if a == nil {
		return fmt.Errorf("app not found: %s", req.Slug)
	}
	rsp.Text = a.Name + " (" + a.Slug + ") by " + a.Author + "\n" + a.Description + "\nTags: " + a.Tags + "\nOpen: /apps/" + a.Slug
	return nil
}

var Spec = service.Spec{
	Name:        "apps",
	Handler:     new(Server),
	Description: "Small self-contained web tools, built and run in place",
	Page:        "/apps",
	Icon:        "apps.svg",
	Card:        service.Glance(Preview),
	Endpoints: map[string]service.Endpoint{
		"Build": {Writes: true, Doc: "Build a small app from a description, save it, and return its details and URL. An app is a single page — a tracker, a checklist, a counter — that keeps its own store and runs in the browser",
			Cost: quota.OpAppBuild, Needs: service.Caller},
		"Read":   {Doc: "Read the details of one app by its slug"},
		"Search": {Doc: "Search the apps directory for small, useful tools, by name, description or tag"},

		// Where an app comes from. Implemented in authoring.go.
		"Create": {Writes: true, Doc: "Create an app — a small, self-contained HTML tool hosted here. Takes the HTML; apps_build writes it for you from a description",
			Cost: quota.OpAppCreate, Needs: service.Caller},
		"Edit": {Writes: true, Doc: "Edit an app you own — its name, description, tags, icon, HTML or price. Fields left out keep their value",
			Cost: quota.OpAppEdit, Needs: service.Caller},
		"Fork":  {Writes: true, Doc: "Fork an app into your own account, to change independently of the original", Needs: service.Caller},
		"Embed": {Needs: service.Caller, Doc: "Get the HTML that puts an app on another page — an iframe tag pointing at the app, which runs there sandboxed the same way it runs here. Apps that charge cannot be embedded, and an app that calls mu. only reaches this instance from a page on it"},
		"Test":  {Writes: true, Doc: "Test an app by checking its HTML and running its mu.api calls server-side, so an author finds out what is broken without opening it", Needs: service.Caller},
	},
}
