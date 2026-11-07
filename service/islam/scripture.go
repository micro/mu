package islam

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

type ReferencesRequest = scripture.SearchRequest
type ReferencesResponse = scripture.ReferencesResponse

// References searches Reminder without requesting a second model's answer.
func (Server) References(ctx context.Context, req *ReferencesRequest, rsp *ReferencesResponse) error {
	result, err := scripture.Default.References(ctx, req.Query)
	if err != nil {
		return err
	}
	*rsp = *result
	return nil
}
