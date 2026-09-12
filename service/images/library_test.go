package images

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"mu/internal/service"
	"mu/internal/userdb"
	"testing"
)

func TestLibraryOwnershipAndMetadata(t *testing.T) {
	ctx := service.WithAccount(context.Background(), "library-owner")
	other := service.WithAccount(context.Background(), "library-other")
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var uploaded ImageResponse
	if err := (Server{}).Upload(ctx, &UploadRequest{Data: base64.StdEncoding.EncodeToString(buf.Bytes()), Description: "lake"}, &uploaded); err != nil {
		t.Fatal(err)
	}
	id := uploaded.Image.ID
	defer userdb.Delete(ns, "library-owner", collection, id)
	if uploaded.Image.Visibility != "private" || uploaded.Image.Source != "uploaded" {
		t.Fatal(uploaded)
	}
	if err := (Server{}).Get(other, &GetRequest{ID: id}, &ImageResponse{}); err == nil {
		t.Fatal("private image leaked")
	}
	title := "Mountain lake"
	tags := []string{"dawn", "water"}
	if err := (Server{}).Update(ctx, &UpdateRequest{ID: id, Title: &title, Tags: &tags}, &ImageResponse{}); err != nil {
		t.Fatal(err)
	}
	var found SearchResponse
	if err := (Server{}).Search(ctx, &SearchRequest{Scope: "mine", Query: "mountain dawn"}, &found); err != nil || len(found.Items) != 1 {
		t.Fatalf("search: %v %#v", err, found)
	}
	if err := (Server{}).Share(ctx, &ShareRequest{ID: id, Public: true}, &ImageResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := (Server{}).Get(other, &GetRequest{ID: id}, &ImageResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := (Server{}).Update(other, &UpdateRequest{ID: id, Title: &title}, &ImageResponse{}); err == nil {
		t.Fatal("foreign update")
	}
	if err := (Server{}).Share(other, &ShareRequest{ID: id, Public: false}, &ImageResponse{}); err == nil {
		t.Fatal("foreign share")
	}
	if err := (Server{}).Delete(other, &GetRequest{ID: id}, &DeleteResponse{}); err == nil {
		t.Fatal("foreign delete")
	}
	if err := (Server{}).Share(ctx, &ShareRequest{ID: id, Public: false}, &ImageResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := (Server{}).Get(other, &GetRequest{ID: id}, &ImageResponse{}); err == nil {
		t.Fatal("unpublished image leaked")
	}
	if err := (Server{}).Delete(ctx, &GetRequest{ID: id}, &DeleteResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := (Server{}).Get(ctx, &GetRequest{ID: id}, &ImageResponse{}); err == nil {
		t.Fatal("deleted record remains")
	}
}
func TestLibraryPaginationPastStoreLimit(t *testing.T) {
	const owner = "image-pages"
	ctx := service.WithAccount(context.Background(), owner)
	defer userdb.DeleteOwner(ns, owner)
	for i := 0; i < 205; i++ {
		if _, err := userdb.Create(ns, owner, collection, map[string]interface{}{"prompt": "page test"}, false); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		var rsp ListResponse
		if err := (Server{}).List(ctx, &ListRequest{Scope: "mine", Limit: 70, Cursor: cursor}, &rsp); err != nil {
			t.Fatal(err)
		}
		for _, item := range rsp.Items {
			if seen[item.ID] {
				t.Fatal("duplicate page item")
			}
			seen[item.ID] = true
		}
		cursor = rsp.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 205 {
		t.Fatalf("listed %d images", len(seen))
	}
	if err := (Server{}).List(context.Background(), &ListRequest{Scope: "mine"}, &ListResponse{}); err == nil {
		t.Fatal("guest mine accepted")
	}
}
