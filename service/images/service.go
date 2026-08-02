package images

import (
	"context"

	"mu/internal/service"
)

// Server is the go-micro service handler for images. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// GenerateRequest is the input for an image generation.
// Generation is metered against the caller's wallet; the account comes from
// the call context, never from the model.
type GenerateRequest struct {
	Prompt string `json:"prompt" description:"What the image should depict"`
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
	rsp.URL = url
	return nil
}

var toolDocs = service.Docs{
	"Generate": "Generate an image from a text prompt and return its URL",
}
