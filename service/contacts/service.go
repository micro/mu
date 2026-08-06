package contacts

import (
	"context"
	"fmt"

	"mu/internal/app"
	"mu/internal/service"
)

// Server is the go-micro service handler for contacts. Its methods are exposed
// as RPC endpoints and, through the agent and gateways, as AI tools — so
// "email Sarah" reaches Find before it reaches mail.
type Server struct{}

func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use contacts")
	}
	return id, nil
}

// ── Add ─────────────────────────────────────────────────────────

type AddRequest struct {
	Name  string `json:"name" description:"The person's name, e.g. \"Sarah Chen\""`
	Email string `json:"email,omitempty" description:"Their email address"`
	Phone string `json:"phone,omitempty" description:"Their phone number"`
	Note  string `json:"note,omitempty" description:"Anything worth remembering about them"`
}

type AddResponse struct {
	Contact *Contact `json:"contact" description:"The stored contact"`
	Result  string   `json:"result" description:"Confirmation"`
}

// Add saves someone to the caller's address book. Adding a name already there
// updates it rather than making a second card.
// @example {"name": "Sarah Chen", "email": "sarah@example.com"}
func (Server) Add(ctx context.Context, req *AddRequest, rsp *AddResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	c, err := Add(owner, req.Name, req.Email, req.Phone, req.Note)
	if err != nil {
		return err
	}
	rsp.Contact = c
	rsp.Result = "Saved " + c.Name + "."
	return nil
}

// ── Find ────────────────────────────────────────────────────────

type FindRequest struct {
	Query string `json:"query" description:"A name, part of a name, or an address"`
}

type FindResponse struct {
	Contacts []*Contact `json:"contacts" description:"Everyone matching, so an ambiguous name can be asked about rather than guessed"`
	Text     string     `json:"text" description:"The same matches as model-ready text"`
}

// Find looks someone up by name or address. Use it before sending mail to a
// person named rather than addressed.
// @example {"query": "sarah"}
func (Server) Find(ctx context.Context, req *FindRequest, rsp *FindResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	own, ext := FindEverywhere(owner, req.Query)
	rsp.Contacts = own

	// Both books in one answer. Two lists would leave the agent choosing
	// between them, and the caller asked "who is Sarah", not "which store".
	rsp.Text = Render(own)
	if len(ext) > 0 {
		if len(own) == 0 {
			rsp.Text = RenderExternal(ext)
		} else {
			rsp.Text += "\n" + RenderExternal(ext)
		}
	}
	rsp.Text += connectHint(owner)
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct{}

type ListResponse struct {
	Contacts []*Contact `json:"contacts" description:"The caller's contacts, by name"`
	Text     string     `json:"text" description:"The same list as model-ready text"`
}

// List returns the caller's whole address book.
// @example {}
func (Server) List(ctx context.Context, _ *ListRequest, rsp *ListResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	rsp.Contacts = List(owner)
	rsp.Text = Render(rsp.Contacts)
	// Deliberately not a dump of the attached book: those run to thousands of
	// entries, most of them auto-collected from mail. It is a place to look
	// names up, not a list to page through.
	if HasExternal(owner) {
		rsp.Text += "\n\n(Your " + ExternalName + " is attached too — ask for a name and it will be searched.)"
	}
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

type DeleteRequest struct {
	ID string `json:"id" description:"The contact's id, from contacts_find or contacts_list"`
}

type DeleteResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Delete removes someone from the caller's address book.
// @example {"id": "0b85f513-9681-44a4-842a-35bbbc4140a8"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	if err := Remove(owner, req.ID); err != nil {
		return err
	}
	rsp.Result = "Deleted."
	return nil
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("contacts", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "contacts",
	Handler:     new(Server),
	Description: "The caller's own address book: turn a name into an address",
	Page:        "/contacts",
	Icon:        "contacts.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		"Add":    {Doc: "Save someone to the address book, or update them if already there"},
		"Find":   {Doc: "Look someone up by name, part of a name, or address"},
		"List":   {Doc: "List the caller's contacts"},
		"Delete": {Doc: "Remove someone from the address book", Destructive: true},
	},
}
