package images

import (
	"context"
	"fmt"

	"strings"

	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/userdb"
)

// Server is the go-micro service handler for images. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// GenerateRequest is the input for an image generation.
// Generation is metered against the caller's wallet; the account comes from
// the call context, never from the model.
type GenerateRequest struct {
	Prompt string `json:"prompt" required:"true" description:"What the image should depict"`
}

// GenerateResponse is the created image.
type GenerateResponse struct {
	Image *Image `json:"image,omitempty"`
	URL   string `json:"url" description:"URL of the generated image"`
}

// Generate creates an image from a text prompt and returns its URL. It costs
// credits, charged to the caller's account; guests cannot generate.
// @example {"prompt": "a calm mountain lake at dawn, soft light"}
func (Server) Generate(ctx context.Context, req *GenerateRequest, rsp *GenerateResponse) error {
	url, err := Generate(service.AccountFrom(ctx), req.Prompt)
	if err != nil {
		return err
	}
	// Generate returns a path relative to this instance, which is what a
	// browser on the same origin needs. A tool call has no origin of its own —
	// the model hands the URL back to whoever asked, on the web, over mail, in
	// chat — so it needs the full address, the same reasoning AbsoluteURL
	// exists for.
	if id, ok := strings.CutPrefix(url, DisplayURL("")); ok {
		record, getErr := userdb.Get(ns, service.AccountFrom(ctx), collection, id)
		if getErr == nil {
			item := recordImage(*record)
			rsp.Image = &item
		}
		url = AbsoluteURL(id)
	}
	rsp.URL = url
	return nil
}

var Spec = service.Spec{
	Name:        "images",
	Handler:     new(Server),
	Description: "Generate and browse images",
	Page:        "/images",
	Scoped:      true,
	Icon:        "images.svg",
	Card:        service.Glance(CardHTML),
	Endpoints: map[string]service.Endpoint{
		"Delete":   {Doc: "Delete an image you own", Destructive: true},
		"Share":    {Doc: "Explicitly publish or unpublish your image", Destructive: true},
		"Update":   {Doc: "Edit metadata on an image you own", Writes: true},
		"Upload":   {Doc: "Upload a private raster image", Writes: true},
		"Daily":    {Doc: "Retrieve the current daily image or a date", Writes: false},
		"Get":      {Doc: "Retrieve accessible image metadata and URL", Writes: false},
		"List":     {Doc: "List stored images or the daily archive with pagination", Writes: false},
		"Generate": {Writes: true, Aliases: []string{"image_generate"}, Doc: "Generate an image from a text prompt and return its URL", Cost: quota.OpImageGenerate},
		"Search":   {Aliases: []string{"image_search"}, Doc: "Search accessible stored images by title, description, prompt and tags; public by default"},
	},
}

// ── Search ──────────────────────────────────────────────────────

// Searching the stock pool was a tool with no service behind it: a closure in
// the assembly calling this package's own Search. The images Spec said an image
// could be generated and said nothing about reusing one already here — which is
// the cheaper answer to most requests, and the one an agent should reach for
// first.

type SearchRequest struct {
	Scope  string `json:"scope" description:"public, mine, all, or daily; defaults to public"`
	Cursor string `json:"cursor"`
	Query  string `json:"query" required:"true" description:"What the image should show, in words"`
	Limit  int    `json:"limit" description:"Max results (default 20)"`
}

type SearchResponse struct {
	Items      []Image `json:"items"`
	NextCursor string  `json:"next_cursor,omitempty"`
	Text       string  `json:"text,omitempty" description:"Matching images: what each depicts, and a URL to reuse"`
}

// Search looks through the public image library — images generated here and
// shared — so an existing one can be reused instead of paying to make another.
// @example {"query": "a lighthouse at dusk"}
func (Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	if strings.TrimSpace(req.Query) == "" {
		return fmt.Errorf("query is required")
	}
	if len(req.Query) > 400 {
		return fmt.Errorf("query exceeds 400 characters")
	}
	page := ListResponse{}
	if err := libraryPage(service.AccountFrom(ctx), ListRequest{Scope: req.Scope, Limit: req.Limit, Cursor: req.Cursor}, req.Query, &page); err != nil {
		return err
	}
	rsp.Items = page.Items
	rsp.NextCursor = page.NextCursor
	var text strings.Builder
	for _, item := range page.Items {
		fmt.Fprintf(&text, "- %s\n  %s\n", item.Description, item.URL)
	}
	rsp.Text = text.String()
	if len(page.Items) == 0 {
		rsp.Text = "No matching images."
	}
	return nil
}
