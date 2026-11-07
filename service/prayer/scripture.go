package prayer

import (
	"context"
	"mu/internal/scripture"
)

type VerseRequest = scripture.VerseRequest
type VerseResponse = scripture.VerseResponse
type SayingRequest = scripture.SayingRequest
type SayingResponse = scripture.SayingResponse

func (Server) Verse(ctx context.Context, req *VerseRequest, rsp *VerseResponse) error {
	return scripture.Default.Verse(ctx, req, rsp)
}
func (Server) Saying(ctx context.Context, req *SayingRequest, rsp *SayingResponse) error {
	return scripture.Default.Saying(ctx, req, rsp)
}

// Compatibility names and response shape for existing Prayer clients.
type SearchRequest = scripture.SearchRequest
type SearchResponse = scripture.SearchResponse

func (Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	return scripture.Default.Search(ctx, req, rsp)
}
