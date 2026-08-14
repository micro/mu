package blog

import (
	"context"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for blog.
type Server struct{}

// ListRequest controls how many posts to return.
type ListRequest struct {
	Limit int `json:"limit" description:"Optional max number of posts (default all recent)"`
}

// ListResponse is a model-ready list of recent posts.
type ListResponse struct {
	Text string `json:"text" description:"Recent blog posts: titles, snippets and ids"`
}

// List returns recent blog posts (titles, snippets and ids).
// @example {}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = RecentText(req.Limit)
	return nil
}

var Spec = service.Spec{
	Name:        "blog",
	Handler:     new(Server),
	Description: "Microblogging with AI-generated daily digests, federated over ActivityPub",
	Page:        "/blog",
	Icon:        "post.png",
	Card:        Preview,
	Endpoints: map[string]service.Endpoint{
		"List": {Doc: "Read recent blog posts — titles, snippets and ids"},

		// One post at a time. Implemented in posts.go.
		"Read": {Doc: "Read one blog post in full, by id or by title. Use after blog_list has found a candidate and the summary is not enough"},
		"Create": {Doc: "Publish a post to the caller's blog. For anything meant to be read later by other people — notes, write-ups, announcements. For a private note to yourself, prefer files or memory",
			Cost: quota.OpBlogCreate, Needs: service.Caller},
		"Update": {Doc: "Edit one of the caller's own posts. Fields left out keep their current value", Needs: service.Caller},
		"Delete": {Doc: "Delete one of the caller's own blog posts, by id or title. Refuses posts written by anyone else, and refuses an ambiguous title rather than guessing. Irreversible, so confirm with the user first",
			Destructive: true, Needs: service.Caller},
	},
	// Create, Update and Delete write the caller's own posts; Read and List are
	// public. Scoped is per service, so it stays false and the three that need
	// a caller get one from the context — see Endpoint.Account.
}
