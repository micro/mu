package images

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/quota"
	"mu/internal/service"
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
	URL string `json:"url" description:"URL of the generated image"`
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
		url = AbsoluteURL(id)
	}
	rsp.URL = url
	return nil
}

var Spec = service.Spec{
	Name:        "images",
	Handler:     new(Server),
	Description: "Image generation, the daily image and its archive",
	Page:        "/images",
	Scoped:      true,
	Icon:        "images.svg",
	Card:        service.Glance(CardHTML),
	Endpoints: map[string]service.Endpoint{
		"Generate": {Writes: true, Aliases: []string{"image_generate"}, Doc: "Generate an image from a text prompt and return its URL", Cost: quota.OpImageGenerate},
		"Search":   {Aliases: []string{"image_search"}, Doc: "Search the public image library by description and get URLs to reuse. Cheaper than generating: look here first"},
	},
}

// ── Search ──────────────────────────────────────────────────────

// Searching the stock pool was a tool with no service behind it: a closure in
// the assembly calling this package's own Search. The images Spec said an image
// could be generated and said nothing about reusing one already here — which is
// the cheaper answer to most requests, and the one an agent should reach for
// first.

type SearchRequest struct {
	Query string `json:"query" required:"true" description:"What the image should show, in words"`
	Limit int    `json:"limit" description:"Max results (default 20)"`
}

type SearchResponse struct {
	Text string `json:"text" description:"Matching images: what each depicts, and a URL to reuse"`
}

// Search looks through the public image library — images generated here and
// shared — so an existing one can be reused instead of paying to make another.
// @example {"query": "a lighthouse at dusk"}
func (Server) Search(_ context.Context, req *SearchRequest, rsp *SearchResponse) error {
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return fmt.Errorf("query is required")
	}
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	recs := Search("", q)
	if len(recs) == 0 {
		rsp.Text = "No matching images in the stock pool."
		return nil
	}
	var b strings.Builder
	for i, rec := range recs {
		if i >= limit {
			break
		}
		prompt, _ := rec.Data["prompt"].(string)
		// Our copy, not the provider's link: the caller is being handed a URL
		// to keep, and the provider's expires.
		b.WriteString(fmt.Sprintf("- %s\n  %s\n", prompt, AbsoluteURL(rec.ID)))
	}
	rsp.Text = b.String()
	return nil
}
