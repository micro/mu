package video

import (
	"context"
	"fmt"
	"mu/internal/data"
	"strings"
)

type ReadRequest struct {
	ID string `json:"id" required:"true" description:"Video id or video_ archive id from List or Search"`
}
type ReadResponse struct {
	Item *Result `json:"item"`
	Text string  `json:"text"`
}

// Read returns metadata already discovered by this instance, without inventing a transcript.
func (Server) Read(_ context.Context, req *ReadRequest, rsp *ReadResponse) error {
	id := strings.TrimPrefix(strings.TrimSpace(req.ID), "video_")
	if !validVideoID.MatchString(id) {
		return fmt.Errorf("invalid video id")
	}
	e := data.ByID("video_" + id)
	if e == nil || e.Type != data.KindVideo || e.Owner != "" {
		return fmt.Errorf("video not found; search for it first")
	}
	field := func(k string) string { v, _ := e.Metadata[k].(string); return v }
	rsp.Item = &Result{ID: id, Type: "video", Title: e.Title, Description: e.Content, URL: "https://youtube.com/watch?v=" + id, Published: data.PostedAt(e), Channel: field("channel"), ChannelID: field("channel_id"), Category: field("category"), Thumbnail: field("thumbnail")}
	rsp.Text = e.Title + " — " + field("channel") + ". Description provided; no transcript."
	return nil
}
