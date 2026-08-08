// Package context is what an agent should know before it starts fetching.
//
// The product had been saying "live context" for a while and meaning a UI
// toggle. The cards are materialised views over the services — headlines,
// movers, unread, today — refreshed on a timer and rendered as text for the
// model. That aggregate is real, it is genuinely the RAG payload, and it was
// reachable from exactly one place: a checkbox next to the input on this
// instance's own chat. An agent connecting over MCP — the audience the whole
// product is named for — could not get it at all.
//
// So it is a tool. One call returns what the caller watches, live, plus what
// has been remembered about them, which is the thing an agent wants at the top
// of a conversation and cannot assemble itself: it would have to call six tools
// to find out what is worth calling a seventh for.
//
// A higher-level primitive, not a service in the catalogue sense. It runs
// nothing of its own — every word it returns came from a service that does —
// so it is headless, like index, and /context is the page a person reads it on.
// Naming it in the services grid would claim a domain it does not own.
package context

import (
	gocontext "context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/memory"
	"mu/internal/service"
)

// Live returns the caller's watched cards as text. Set by main.go from the home
// package, which owns the cards — a hook because home imports half the services
// and nothing in service/ may import it back.
var Live func(accountID string) string

// Server is the go-micro handler. Its exported methods become the context_* tools.
type Server struct{}

func caller(ctx gocontext.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to read your context")
	}
	return id, nil
}

// ── Get ─────────────────────────────────────────────────────────

type GetRequest struct {
	Include string `json:"include,omitempty" description:"\"live\", \"remembered\", or omit for both"`
}

type GetResponse struct {
	Live       string `json:"live,omitempty" description:"What the caller watches, as it stands now"`
	Remembered string `json:"remembered,omitempty" description:"What has been remembered about the caller across conversations"`
	Empty      bool   `json:"empty,omitempty" description:"True when there is no context at all — nothing watched and nothing remembered"`
}

// Get returns what is worth knowing about the caller right now.
// @example {}
func (Server) Get(ctx gocontext.Context, req *GetRequest, rsp *GetResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	want := strings.ToLower(strings.TrimSpace(req.Include))
	if want == "" || want == "live" {
		if Live != nil {
			rsp.Live = Live(owner)
		}
	}
	if want == "" || want == "remembered" {
		var b strings.Builder
		for _, e := range memory.All(owner) {
			b.WriteString("- " + e.Key + ": " + e.Value + "\n")
		}
		rsp.Remembered = strings.TrimSpace(b.String())
	}
	rsp.Empty = rsp.Live == "" && rsp.Remembered == ""
	return nil
}

// LoadService registers context as a service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("context", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "context",
	Handler:     new(Server),
	Description: "What an agent should know before it starts fetching",
	// Headless: it runs nothing of its own, and /context is where a person
	// reads it. Scoped, because it is an aggregate of personal things and an
	// agent granted "news" should not get the unread count through the back.
	Scoped: true,
	Endpoints: map[string]service.Endpoint{
		"Get": {Doc: "Read what the caller watches, live, and what has been remembered about them. Call it once at the start of a conversation instead of guessing which of the other tools to try"},
	},
}
