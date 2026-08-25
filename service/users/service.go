package users

// The service: three questions whose answers do not depend on who is asking.
//
// Scoped is false, which is the whole point. Every other account-shaped service
// here — contacts, mail, notes — answers about the caller, and this answers
// about the instance. An agent asking "who else is here" gets the same list
// whoever it is asking for, which is what makes these tools rather than
// account furniture.
//
// Nothing writes. A directory is a view of accounts, and an account is changed
// where accounts are changed: by the person at /account, or by an admin at
// /admin/users. A users_ban or users_rename would be a second door onto
// somebody's identity, reachable by a model, which is the shape AGENTS.md's
// destructive rule exists to keep out — and here it is easier to simply not
// build it.

import (
	"context"
	"fmt"

	"mu/internal/app"
	"mu/internal/service"
)

// Server is the handler go-micro derives the tools from.
type Server struct{}

var Spec = service.Spec{
	Name:        "users",
	Handler:     new(Server),
	Description: "Who is on this instance: the people and the agents, and whether they are here now",
	Page:        "/users",
	Icon:        "account.png",
	// Needs an account, all three, and the page needs a session for the same
	// reason: each /@name page is public and always has been, but being able to
	// *enumerate* them is what makes a directory worth scraping, and a tool is
	// more scriptable than a page. A caller who has an account is somebody this
	// instance already knows.
	Endpoints: map[string]service.Endpoint{
		"List": {Needs: service.Account,
			Doc: "Everyone on this instance, online first and then newest. " +
				"Agents are included and marked; an agent account is a user here like any other"},
		"Find": {Needs: service.Account,
			Doc: "Look somebody up by username or display name. " +
				"Use this to turn a name somebody mentioned into an address you can write to"},
		"Get": {Needs: service.Account, Doc: "One user, by username"},
	},
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct {
	Online bool `json:"online,omitempty" description:"Only those seen in the last three minutes"`
	Limit  int  `json:"limit,omitempty" description:"How many to return, newest first. Default 100"`
}

type ListResponse struct {
	Users []User `json:"users" description:"The directory"`
	Total int    `json:"total" description:"How many accounts there are in all"`
	Text  string `json:"text" description:"The same thing as a sentence"`
}

// List is everyone here.
// @example {"online": true}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	all := List()
	rsp.Total = len(all)

	if req.Online {
		var on []User
		for _, u := range all {
			if u.Profile.Online {
				on = append(on, u)
			}
		}
		all = on
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if len(all) > limit {
		all = all[:limit]
	}
	rsp.Users = all
	rsp.Text = describe(all, rsp.Total, req.Online)
	return nil
}

// ── Find ────────────────────────────────────────────────────────

type FindRequest struct {
	Query string `json:"query" required:"true" description:"A username or part of a display name"`
}

type FindResponse struct {
	Users []User `json:"users" description:"Everyone matching"`
	Text  string `json:"text" description:"The same thing as a sentence"`
}

// Find looks somebody up.
// @example {"query": "asim"}
func (Server) Find(ctx context.Context, req *FindRequest, rsp *FindResponse) error {
	rsp.Users = Find(req.Query)
	if len(rsp.Users) == 0 {
		rsp.Text = "Nobody here matches " + req.Query + "."
		return nil
	}
	rsp.Text = describe(rsp.Users, len(rsp.Users), false)
	return nil
}

// ── Get ─────────────────────────────────────────────────────────

type GetRequest struct {
	ID string `json:"id" required:"true" description:"Their username"`
}

type GetResponse struct {
	User User   `json:"user" description:"Them"`
	Text string `json:"text" description:"The same thing as a sentence"`
}

// Get is one user.
// @example {"id": "asim"}
func (Server) Get(ctx context.Context, req *GetRequest, rsp *GetResponse) error {
	u, ok := Get(req.ID)
	if !ok {
		return fmt.Errorf("nobody here is called %q", req.ID)
	}
	rsp.User = u
	rsp.Text = line(u)
	return nil
}

// describe is the list as prose, because a model reading a hundred JSON objects
// answers better from a sentence than from the array it also has.
func describe(list []User, total int, onlineOnly bool) string {
	if len(list) == 0 {
		if onlineOnly {
			return "Nobody is online right now."
		}
		return "There is nobody here yet."
	}
	out := ""
	for _, u := range list {
		out += "- " + line(u) + "\n"
	}
	head := fmt.Sprintf("%d of %d", len(list), total)
	if onlineOnly {
		head = fmt.Sprintf("%d online, of %d", len(list), total)
	}
	return head + " on this instance:\n" + out
}

// line is one user as a sentence.
func line(u User) string {
	name := u.Display()
	if name != u.ID {
		name += " (" + u.ID + ")"
	}
	what := "person"
	if u.Account.Agent {
		what = "agent"
	}
	out := name + " — " + what
	if u.Profile.Online {
		out += ", online now"
	}
	if u.Profile.Status != "" {
		out += ", " + u.Profile.Status
	}
	return out
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("users", "service register failed: %v", err)
	}
}
