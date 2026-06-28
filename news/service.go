package news

import "context"

// Server is the go-micro service handler for news. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// HeadlinesRequest filters the headline list.
type HeadlinesRequest struct {
	Topic string `json:"topic" description:"Optional topic/category filter (e.g. tech, world, business)"`
	Limit int    `json:"limit" description:"Optional max number of headlines (default 30)"`
}

// HeadlinesResponse is a model-ready list of headlines.
type HeadlinesResponse struct {
	Text string `json:"text" description:"Recent headlines with short summaries, balanced across topics"`
}

// Headlines returns recent news headlines with short summaries, balanced across
// topics (not dominated by one topic like crypto).
// @example {"topic": "tech"}
func (Server) Headlines(_ context.Context, req *HeadlinesRequest, rsp *HeadlinesResponse) error {
	rsp.Text = HeadlinesText(req.Topic, req.Limit)
	return nil
}

// ReadRequest selects one article.
type ReadRequest struct {
	ID string `json:"id" description:"Article id (from Headlines) or article URL"`
}

// ReadResponse is the full article text.
type ReadResponse struct {
	Text string `json:"text" description:"Title, source, summary and body of the article"`
}

// Read reads one news article in full by its id (from Headlines) or by URL.
// @example {"id": "https://example.com/article"}
func (Server) Read(_ context.Context, req *ReadRequest, rsp *ReadResponse) error {
	text, err := ArticleText(req.ID)
	rsp.Text = text
	return err
}
