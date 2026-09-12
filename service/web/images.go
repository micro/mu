package web

import (
	"context"
	"mu/internal/imagesearch"
)

type ImagesRequest struct {
	Query string `json:"query" required:"true" description:"Images to find on the web"`
	Limit int    `json:"limit" description:"Maximum results, default 24"`
}
type WebImage struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
	SourceURL string `json:"source_url"`
	Source    string `json:"source"`
}
type ImagesResponse struct {
	Items []WebImage `json:"items"`
}

func (Server) Images(ctx context.Context, req *ImagesRequest, rsp *ImagesResponse) error {
	rows, err := imagesearch.Search(ctx, req.Query)
	if err != nil {
		return err
	}
	limit := req.Limit
	if limit <= 0 || limit > 24 {
		limit = 24
	}
	rsp.Items = []WebImage{}
	for _, row := range rows {
		if len(rsp.Items) == limit {
			break
		}
		rsp.Items = append(rsp.Items, WebImage{Title: row.Title, URL: row.Properties.URL, Thumbnail: row.Thumbnail.Src, SourceURL: row.URL, Source: row.Source})
	}
	return nil
}
