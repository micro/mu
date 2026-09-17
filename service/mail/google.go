package mail

import (
	"context"
	"mu/internal/google"
	"mu/internal/service"
)

type GmailSearchRequest struct {
	Query string `json:"query" description:"Gmail search, e.g. is:unread. Limited to the last seven days. Returns headers only; use GmailRead for a relevant message body. Older mail is unavailable"`
	Page  string `json:"page,omitempty" description:"Next page token from a previous search"`
	Limit int    `json:"limit,omitempty" description:"Number of messages, up to 20"`
}
type GmailReadRequest struct {
	ID string `json:"id" required:"true" description:"Gmail message id from GmailSearch"`
}

// GmailSearch searches the caller's connected Gmail, read-only and on demand.
func (Server) GmailSearch(ctx context.Context, req *GmailSearchRequest, rsp *google.GmailPage) error {
	result, err := google.SearchGmail(ctx, service.AccountFrom(ctx), req.Query, req.Page, req.Limit)
	*rsp = result
	return err
}

// GmailRead reads a recent connected Gmail message without marking it read or sending mail.
func (Server) GmailRead(ctx context.Context, req *GmailReadRequest, rsp *google.GmailMessage) error {
	result, err := google.ReadGmail(ctx, service.AccountFrom(ctx), req.ID)
	*rsp = result
	return err
}
