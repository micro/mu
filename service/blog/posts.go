package blog

// Reading and writing one post.
//
// blog_read, blog_create, blog_update and blog_delete were tools with no
// service behind them: registered in internal/api against GET, POST, PATCH and
// DELETE on /blog/post, so calling one built an HTTP request and pushed it
// through the mux to arrive at the page handler. The blog Spec declared exactly
// one endpoint, List, and everything else a caller could do was reachable only
// by naming a URL.
//
// The rules they enforce are the ones the page enforces, because they are the
// rules: you may edit and delete your own posts and nobody else's. What is new
// is that they are stated once, in Go, where the answer does not depend on
// which door the request arrived at.
//
// Finding a post by title is here too. An agent usually has a name rather than
// an id, and an ambiguous name is refused rather than guessed — deleting the
// wrong post is not recoverable, and neither is overwriting one.

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/auth"
	"mu/internal/service"
)

func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use the blog")
	}
	return id, nil
}

// find resolves a post from an id or a title. It is deliberately strict about
// titles: one match or none.
func find(id, title string) (*Post, error) {
	if id != "" {
		if p := GetPost(id); p != nil {
			return p, nil
		}
		return nil, fmt.Errorf("no post with id %q", id)
	}
	want := strings.ToLower(strings.TrimSpace(title))
	if want == "" {
		return nil, fmt.Errorf("give an id or a title")
	}

	mutex.RLock()
	var hits []*Post
	for _, p := range posts {
		if p != nil && strings.Contains(strings.ToLower(p.Title), want) {
			hits = append(hits, p)
		}
	}
	mutex.RUnlock()
	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("no post matching %q", title)
	case 1:
		return hits[0], nil
	}
	names := make([]string, 0, len(hits))
	for _, p := range hits {
		names = append(names, fmt.Sprintf("%q (%s)", p.Title, p.ID))
	}
	return nil, fmt.Errorf("%q matches %d posts — name one by id: %s",
		title, len(hits), strings.Join(names, ", "))
}

// mine refuses a post the caller did not write. Ownership is by account id;
// the author's display name is not an identity.
func mine(p *Post, who string) error {
	if p.AuthorID != who {
		return fmt.Errorf("that post was written by somebody else")
	}
	return nil
}

// ── Read ────────────────────────────────────────────────────────

type ReadRequest struct {
	ID    string `json:"id" description:"The post's id, as given by blog_list"`
	Title string `json:"title" description:"The post's title, or enough of it to be unambiguous — use this when you have a name rather than an id"`
}

type ReadResponse struct {
	Text string `json:"text" description:"The post in full: title, author, when it was published, and the body"`
}

// Read returns one post in full, by id or by title. Use it after blog_list or
// blog_list has found a candidate and the summary is not enough.
// @example {"title": "on writing"}
func (Server) Read(_ context.Context, req *ReadRequest, rsp *ReadResponse) error {
	p, err := find(req.ID, req.Title)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(p.Title + "\n")
	b.WriteString("by " + p.Author + ", " + p.CreatedAt.Format("2 January 2006") + "\n\n")
	b.WriteString(p.Content)
	rsp.Text = b.String()
	return nil
}

// ── Create ──────────────────────────────────────────────────────

type CreateRequest struct {
	Content string `json:"content" required:"true" description:"The post body, at least 50 characters"`
	Title   string `json:"title" description:"Post title. One is generated from the body if omitted"`
	Tags    string `json:"tags" description:"Comma-separated tags"`
	Private bool   `json:"private" description:"True to keep it to yourself"`
}

type CreateResponse struct {
	Result string `json:"result" description:"Confirmation, with the post's URL"`
}

// Create publishes a post to the caller's blog. For anything meant to be read
// later by other people; for a private note, prefer files or memory.
// @example {"title": "On writing", "content": "..."}
func (Server) Create(ctx context.Context, req *CreateRequest, rsp *CreateResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(req.Content)) < 50 {
		return fmt.Errorf("a post needs at least 50 characters")
	}
	name := who
	if acc, err := auth.GetAccount(who); err == nil && acc.Name != "" {
		name = acc.Name
	}
	if err := CreatePost(req.Title, req.Content, name, who, req.Tags, req.Private); err != nil {
		return err
	}
	rsp.Result = "Published."
	return nil
}

// ── Update ──────────────────────────────────────────────────────

type UpdateRequest struct {
	ID      string `json:"id" required:"true" description:"The post's id, as given by blog_list"`
	Title   string `json:"title" description:"New title. Left alone if omitted"`
	Content string `json:"content" description:"New body, at least 50 characters. Left alone if omitted"`
	Tags    string `json:"tags" description:"New comma-separated tags. Left alone if omitted"`
}

type UpdateResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Update edits one of the caller's own posts. Fields left out keep their
// current value, so this can change a title without resending the body.
// @example {"id": "abc123", "title": "A better title"}
func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *UpdateResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	p, err := find(req.ID, "")
	if err != nil {
		return err
	}
	if err := mine(p, who); err != nil {
		return err
	}

	title, content, tags := p.Title, p.Content, p.Tags
	if req.Title != "" {
		title = req.Title
	}
	if req.Content != "" {
		if len(strings.TrimSpace(req.Content)) < 50 {
			return fmt.Errorf("a post needs at least 50 characters")
		}
		content = req.Content
	}
	if req.Tags != "" {
		tags = req.Tags
	}
	if err := UpdatePost(p.ID, title, content, tags, p.Private); err != nil {
		return err
	}
	rsp.Result = "Updated."
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

type DeleteRequest struct {
	ID    string `json:"id" description:"The post's id, as given by blog_list"`
	Title string `json:"title" description:"The post's title, or enough of it to be unambiguous. An ambiguous title is refused rather than guessed — deleting the wrong post is not recoverable"`
}

type DeleteResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Delete removes one of the caller's own posts. Irreversible, so confirm with
// the user first.
// @example {"id": "abc123"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	p, err := find(req.ID, req.Title)
	if err != nil {
		return err
	}
	if err := mine(p, who); err != nil {
		return err
	}
	if err := DeletePost(p.ID); err != nil {
		return err
	}
	rsp.Result = "Deleted " + p.Title + "."
	return nil
}
