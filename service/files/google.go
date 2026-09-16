package files

import (
	"context"
	"mu/internal/google"
	"mu/internal/service"
)

type DriveSearchRequest struct {
	Query string `json:"query" description:"Text to find in the caller's connected Google Drive"`
	Page  string `json:"page,omitempty" description:"Next page token from a previous search"`
	Limit int    `json:"limit,omitempty" description:"Number of files, up to 20"`
}
type DriveReadRequest struct {
	ID string `json:"id" required:"true" description:"Drive file id from DriveSearch"`
}

// DriveSearch searches the caller's connected Google Drive, read-only and on demand.
func (Server) DriveSearch(ctx context.Context, req *DriveSearchRequest, rsp *google.DrivePage) error {
	result, err := google.SearchDrive(ctx, service.AccountFrom(ctx), req.Query, req.Page, req.Limit)
	*rsp = result
	return err
}

// DriveRead reads text files or exports Google Docs, Sheets and Slides as text.
func (Server) DriveRead(ctx context.Context, req *DriveReadRequest, rsp *google.DriveContent) error {
	result, err := google.ReadDrive(ctx, service.AccountFrom(ctx), req.ID)
	*rsp = result
	return err
}
