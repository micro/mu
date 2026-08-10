// Package cache is a key and a value that survive the conversation.
//
// Named for its shape rather than its lifetime, and the distinction is worth
// stating because the name invites the wrong reading: nothing here expires and
// nothing is evicted. It is a key and a value, set and read and deleted by
// name, which is what separates it from db — named collections of records that
// are queried. Both are durable; one is addressed and one is searched.
//
// It is a store, not a dossier. Whatever a caller puts under a label comes back
// under that label — a place, a tone, a project id, an API's page cursor. It
// was described as "what an agent knows about you", which is one thing people
// use it for and not what it is, and a description that names one use narrows
// what anybody else tries.
//
// Nothing expires and nothing is evicted, despite the name. What separates it
// from db is shape: this is addressed by key, db holds named collections you
// query. Both are durable.
package cache

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/cache"
	"mu/internal/service"
)

// Server is the go-micro handler. Its exported methods become the memory_* tools.
type Server struct{}

// caller resolves the authenticated account from call metadata. No owner field
// on any request here, deliberately: an argument can be chosen by whoever makes
// the call, context metadata cannot — and this is the most personal store on
// the instance.
func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use memory")
	}
	return id, nil
}

// ── Set ─────────────────────────────────────────────────────────

type SetRequest struct {
	Key   string `json:"key" required:"true" description:"A short label, e.g. \"location\" or \"tone\". Writing to a label that exists replaces it"`
	Value string `json:"value" required:"true" description:"What to remember, in a sentence"`
}

type SetResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Set remembers something about the caller.
// @example {"key": "location", "value": "London"}
func (Server) Set(ctx context.Context, req *SetRequest, rsp *SetResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	if req.Key == "" || req.Value == "" {
		return fmt.Errorf("a memory needs a key and a value")
	}
	cache.Set(owner, req.Key, req.Value)
	rsp.Result = "remembered"
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct{}

type Note struct {
	Key   string `json:"key" description:"The label"`
	Value string `json:"value" description:"What is remembered"`
}

type ListResponse struct {
	Notes []Note `json:"notes" description:"Everything remembered about the caller"`
}

// List returns everything remembered about the caller.
// @example {}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	for _, e := range cache.All(owner) {
		rsp.Notes = append(rsp.Notes, Note{Key: e.Key, Value: e.Value})
	}
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

type DeleteRequest struct {
	Key string `json:"key" required:"true" description:"The label to forget, as returned by memory_list"`
}

type DeleteResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Delete forgets one thing.
// @example {"key": "location"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	if req.Key == "" {
		return fmt.Errorf("say which key to forget")
	}
	cache.Delete(owner, req.Key)
	rsp.Result = "forgotten"
	return nil
}

// LoadService registers memory as a service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("memory", "service register failed: %v", err)
	}
}

// ── Get ─────────────────────────────────────────────────────────

// GetRequest reads one label.
type GetRequest struct {
	Key string `json:"key" required:"true" description:"The label to read, as given to cache_set"`
}

// GetResponse is the stored value.
type GetResponse struct {
	Value string `json:"value" description:"What is stored under that label, or empty if nothing is"`
	Text  string `json:"text" description:"The label and its value, or a note that nothing is stored under it"`
}

// Get reads one label.
//
// It was missing: a caller could write a label and then only read the whole
// list back to find it, which is the wrong shape for a key-value store and gets
// worse the more labels there are.
// @example {"key": "location"}
func (Server) Get(ctx context.Context, req *GetRequest, rsp *GetResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to use the cache")
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return fmt.Errorf("key is required")
	}
	rsp.Value = cache.Get(who, key)
	if rsp.Value == "" {
		rsp.Text = "Nothing stored under " + key + "."
		return nil
	}
	rsp.Text = key + ": " + rsp.Value
	return nil
}

var Spec = service.Spec{
	Name:        "cache",
	Icon:        "cache.svg",
	Handler:     new(Server),
	Description: "What an agent knows about you between conversations",
	// Headless. The Memory card on /account is where a person reads and edits
	// this; the service is how an agent does.
	Scoped: true,
	Endpoints: map[string]service.Endpoint{
		"Get":    {Doc: "Read one label's value. Use it when you know the label; cache_list when you do not"},
		"Set":    {Aliases: []string{"memory_set"}, Doc: "Store a value under a label, so it is there next conversation. Writing to a label that exists replaces it"},
		"List":   {Aliases: []string{"memory_list"}, Doc: "List every label the caller has stored, with its value"},
		"Delete": {Aliases: []string{"memory_delete"}, Doc: "Delete one label and its value", Destructive: true},
	},
}
