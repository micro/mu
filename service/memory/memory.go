// Package memory is what an agent knows about you between conversations.
//
// The store already existed, in internal/memory, and it was invisible from both
// sides. A person could not see it — that was the /context page. An agent could
// not use it: facts were extracted from what you said by a background model
// call and written behind everyone's back, and there was no tool to remember
// something on purpose, no tool to ask what was already known, and no tool to
// correct a wrong note. An agent that cannot say "remember this" has to be told
// the same thing every session, which is the exact failure persistent memory
// exists to prevent.
//
// So it is a service, and it passes the test the others pass: an agent arrives
// holding a model, and does not arrive holding what you told it last Tuesday.
// That is worth running.
//
// Headless, like index. The store is one thing; /context is a lens over it that
// also shows what you have put in front of your agents and what they can go and
// fetch, and a second page listing the same notes would be a second place for
// the same truth.
package memory

import (
	"context"
	"fmt"

	"mu/internal/app"
	"mu/internal/memory"
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
	memory.Set(owner, req.Key, req.Value)
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
	for _, e := range memory.All(owner) {
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
	memory.Delete(owner, req.Key)
	rsp.Result = "forgotten"
	return nil
}

// LoadService registers memory as a service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("memory", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "memory",
	Handler:     new(Server),
	Description: "What an agent knows about you between conversations",
	// Headless. /context is the lens over this, and it shows more than the
	// notes — what you watch, and what an agent can go and fetch.
	Scoped: true,
	Endpoints: map[string]service.Endpoint{
		"Set":    {Doc: "Remember something about the caller across conversations. Use it when they say to remember something, or state a durable preference or fact"},
		"List":   {Doc: "Read everything remembered about the caller. Use it before asking them something they may already have said"},
		"Delete": {Doc: "Forget one thing, by its key", Destructive: true},
	},
}
