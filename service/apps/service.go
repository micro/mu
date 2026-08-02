package apps

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/service"
	"mu/service/wallet"
)

// Server is the go-micro service handler for apps.
type Server struct{}

// BuildRequest describes an app to generate. The owner is the authenticated
// caller, taken from the call context — never a model-supplied author.
type BuildRequest struct {
	Prompt string `json:"prompt" description:"Description of the app to build"`
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
	a, err := BuildMicroApp(req.Prompt, account, AuthorNameFor(account))
	if err != nil {
		return err
	}
	rsp.Name = a.Name
	rsp.Slug = a.Slug
	rsp.URL = "/apps/" + a.Slug
	rsp.Run = "/apps/" + a.Slug + "/run"
	return nil
}

// AppSearchRequest searches the apps directory.
type AppSearchRequest struct {
	Query string `json:"query" description:"Search query (name, description or tag)"`
}

// AppSearchResponse is a model-ready list of matching apps.
type AppSearchResponse struct {
	Text string `json:"text" description:"Matching apps: name, slug and description"`
}

// Search searches the apps directory for small, useful tools.
// @example {"query": "tracker"}
func (Server) Search(_ context.Context, req *AppSearchRequest, rsp *AppSearchResponse) error {
	results := SearchApps(req.Query)
	if len(results) == 0 {
		rsp.Text = "No apps found for \"" + req.Query + "\"."
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
	Slug string `json:"slug" description:"App slug"`
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
	Endpoints: map[string]service.Endpoint{
		"Build":  {Doc: "Generate a small app (tracker, checklist or counter) from a description", Cost: wallet.OpAppBuild},
		"Read":   {Doc: "Read the details of one app by its slug"},
		"Search": {Doc: "Search the apps directory for small, useful tools"},
	},
}
