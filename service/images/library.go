package images

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mu/internal/app"
	"mu/internal/blob"
	"mu/internal/service"
	"mu/internal/userdb"
	"strconv"
	"strings"
)

type Image struct {
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Prompt      string   `json:"prompt,omitempty"`
	Source      string   `json:"source"`
	Visibility  string   `json:"visibility"`
	CreatedAt   string   `json:"created_at"`
}

func recordImage(r userdb.Record) Image {
	text := func(k string) string { s, _ := r.Data[k].(string); return s }
	tags := []string{}
	raw, _ := json.Marshal(r.Data["tags"])
	_ = json.Unmarshal(raw, &tags)
	if tags == nil {
		tags = []string{}
	}
	description := text("description")
	if description == "" {
		description = text("prompt")
	}
	source := "generated"
	if r.Data["uploaded"] == true {
		source = "uploaded"
	}
	visibility := "private"
	if r.Public {
		visibility = "public"
	}
	return Image{ID: r.ID, URL: AbsoluteURL(r.ID), Title: text("title"), Description: description, Tags: tags, Prompt: text("prompt"), Source: source, Visibility: visibility, CreatedAt: r.Created.Format("2006-01-02T15:04:05Z07:00")}
}
func dailyImage(d Daily) Image {
	return Image{ID: "daily:" + d.Date, URL: app.PublicURL() + "/images/daily/" + d.Date, Title: d.Theme, Description: d.Prompt, Tags: []string{d.Theme}, Prompt: d.Prompt, Source: "daily", Visibility: "public", CreatedAt: d.Date}
}

type ListRequest struct {
	Scope  string `json:"scope" description:"mine, public, all (own plus public), or daily; defaults to public"`
	Limit  int    `json:"limit" description:"Page size, default 20, maximum 100"`
	Cursor string `json:"cursor" description:"Next cursor from the previous page; keep query and scope unchanged"`
}
type ListResponse struct {
	Items      []Image `json:"items"`
	NextCursor string  `json:"next_cursor,omitempty"`
}
type GetRequest struct {
	ID string `json:"id" required:"true"`
}
type ImageResponse struct {
	Image Image `json:"image"`
}
type DailyRequest struct {
	Date string `json:"date" description:"YYYY-MM-DD; omitted for the current daily image"`
}

func (Server) Daily(_ context.Context, req *DailyRequest, rsp *ImageResponse) error {
	if req.Date == "" {
		d := getDaily()
		if d.Date == "" {
			return userdb.ErrNotFound
		}
		rsp.Image = dailyImage(d)
		return nil
	}
	if !validDate(req.Date) {
		return fmt.Errorf("invalid date")
	}
	for _, d := range Archive(0) {
		if d.Date == req.Date {
			rsp.Image = dailyImage(d)
			return nil
		}
	}
	return userdb.ErrNotFound
}
func (s Server) Get(ctx context.Context, req *GetRequest, rsp *ImageResponse) error {
	if strings.HasPrefix(req.ID, "daily:") {
		return s.Daily(ctx, &DailyRequest{Date: strings.TrimPrefix(req.ID, "daily:")}, rsp)
	}
	r, err := userdb.Get(ns, service.AccountFrom(ctx), collection, req.ID)
	if err != nil {
		return err
	}
	rsp.Image = recordImage(*r)
	return nil
}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	return libraryPage(service.AccountFrom(ctx), *req, "", rsp)
}
func libraryPage(caller string, req ListRequest, query string, rsp *ListResponse) error {
	scope := req.Scope
	if scope == "" {
		scope = "public"
	}
	if scope != "mine" && scope != "public" && scope != "all" && scope != "daily" {
		return fmt.Errorf("invalid scope")
	}
	if scope == "mine" && caller == "" {
		return userdb.ErrAuth
	}
	offset := 0
	if req.Cursor != "" {
		n, err := strconv.Atoi(req.Cursor)
		if err != nil || n < 0 || n > 10000000 {
			return fmt.Errorf("invalid cursor")
		}
		offset = n
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rsp.Items = []Image{}
	rsp.NextCursor = ""
	matches := func(i Image) bool {
		hay := strings.ToLower(i.Title + " " + i.Description + " " + i.Prompt + " " + strings.Join(i.Tags, " "))
		for _, word := range strings.Fields(strings.ToLower(query)) {
			if !strings.Contains(hay, word) {
				return false
			}
		}
		return true
	}
	if scope == "daily" {
		all := Archive(0)
		for i := offset; i < len(all); i++ {
			item := dailyImage(all[i])
			if matches(item) {
				rsp.Items = append(rsp.Items, item)
			}
			if len(rsp.Items) == limit {
				if i+1 < len(all) {
					rsp.NextCursor = strconv.Itoa(i + 1)
				}
				break
			}
		}
		return nil
	}
	for {
		rows, more, err := userdb.Page(ns, caller, collection, scope, offset, 100)
		if err != nil {
			return err
		}
		for idx, r := range rows {
			item := recordImage(r)
			if matches(item) {
				rsp.Items = append(rsp.Items, item)
			}
			if len(rsp.Items) == limit {
				if idx+1 < len(rows) || more {
					rsp.NextCursor = strconv.Itoa(offset + idx + 1)
				}
				return nil
			}
		}
		offset += len(rows)
		if !more {
			return nil
		}
	}
}

type UpdateRequest struct {
	ID          string    `json:"id" required:"true"`
	Title       *string   `json:"title,omitempty"`
	Description *string   `json:"description,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
}

func owned(caller string, id string) (*userdb.Record, error) {
	if caller == "" {
		return nil, userdb.ErrAuth
	}
	r, err := userdb.Get(ns, caller, collection, id)
	if err != nil {
		return nil, err
	}
	if r.Owner != caller {
		return nil, userdb.ErrNotFound
	}
	return r, nil
}
func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *ImageResponse) error {
	r, err := owned(service.AccountFrom(ctx), req.ID)
	if err != nil {
		return err
	}
	if req.Title != nil {
		if len(*req.Title) > 200 {
			return fmt.Errorf("title exceeds 200 characters")
		}
		r.Data["title"] = *req.Title
	}
	if req.Description != nil {
		if len(*req.Description) > 4000 {
			return fmt.Errorf("description exceeds 4000 characters")
		}
		r.Data["description"] = *req.Description
	}
	if req.Tags != nil {
		if len(*req.Tags) > 30 {
			return fmt.Errorf("too many tags")
		}
		for _, tag := range *req.Tags {
			if len(tag) > 80 {
				return fmt.Errorf("tag too long")
			}
		}
		r.Data["tags"] = *req.Tags
	}
	r, err = userdb.Update(ns, r.Owner, collection, r.ID, r.Data, r.Public)
	if err == nil {
		rsp.Image = recordImage(*r)
	}
	return err
}

type ShareRequest struct {
	ID     string `json:"id" required:"true"`
	Public bool   `json:"public" description:"Explicitly publish or unpublish this image"`
}

func (Server) Share(ctx context.Context, req *ShareRequest, rsp *ImageResponse) error {
	r, err := owned(service.AccountFrom(ctx), req.ID)
	if err != nil {
		return err
	}
	r, err = userdb.Update(ns, r.Owner, collection, r.ID, r.Data, req.Public)
	if err == nil {
		rsp.Image = recordImage(*r)
	}
	return err
}

type DeleteResponse struct {
	Deleted bool `json:"deleted"`
}

func (Server) Delete(ctx context.Context, req *GetRequest, rsp *DeleteResponse) error {
	r, err := owned(service.AccountFrom(ctx), req.ID)
	if err != nil {
		return err
	}
	if err = userdb.Delete(ns, r.Owner, collection, r.ID); err != nil {
		return err
	}
	if key, ok := r.Data["file"].(string); ok && key != "" {
		if err = blob.Delete(key); err != nil {
			return fmt.Errorf("record deleted but image bytes could not be removed: %w", err)
		}
	}
	rsp.Deleted = true
	return nil
}

type UploadRequest struct {
	Data        string `json:"data" required:"true" description:"Base64 PNG, JPEG or GIF; at most 512 KB decoded and 8 megapixels; browser uploads support 8 MB"`
	Description string `json:"description" description:"Image caption, at most 1000 characters"`
}

func (Server) Upload(ctx context.Context, req *UploadRequest, rsp *ImageResponse) error {
	caller := service.AccountFrom(ctx)
	if caller == "" {
		return userdb.ErrAuth
	}
	if len(req.Data) > base64.StdEncoding.EncodedLen(512<<10) {
		return fmt.Errorf("API image exceeds 512 KB; use the browser upload for larger images")
	}
	raw, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		return fmt.Errorf("invalid base64 image")
	}
	r, err := saveUpload(caller, raw, req.Description)
	if err == nil {
		rsp.Image = recordImage(*r)
	}
	return err
}
