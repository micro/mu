package images

import "context"

// Server is the go-micro service handler for images. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// GenerateRequest is the input for an image generation.
type GenerateRequest struct {
	// AccountID is supplied by the platform (the caller's account), not the
	// model — image generation is metered against this account's wallet.
	AccountID string `json:"account_id" description:"Caller account (supplied by the platform)"`
	Prompt    string `json:"prompt" description:"What the image should depict"`
}

// GenerateResponse is the created image.
type GenerateResponse struct {
	URL string `json:"url" description:"URL of the generated image"`
}

// Generate creates an image from a text prompt and returns its URL. It costs
// credits, charged to the caller's account; guests cannot generate.
// @example {"prompt": "a calm mountain lake at dawn, soft light"}
func (Server) Generate(_ context.Context, req *GenerateRequest, rsp *GenerateResponse) error {
	url, err := Generate(req.AccountID, req.Prompt)
	if err != nil {
		return err
	}
	rsp.URL = url
	return nil
}
