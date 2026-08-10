// Package docs is the caller's own documents: named collections of JSON
// records, private by default, that outlive any one conversation.
//
// It was deliberately not a service once. The reasoning was a house — a house
// has a mailbox, a calendar and a shelf of files, and it does not have a
// database — and under a product for people that was right. It is not right for
// a product whose first sentence is tools for agents. An agent without storage
// re-derives what it already worked out, asks again for what it was already
// told, and cannot continue anything across two runs. A database is not
// furniture in that house; it is the floor.
//
// Two things made the old arrangement cost something real rather than reading
// oddly. Tools with no service behind them are outside the scoping model, so
// docs_* was unreachable by every scoped agent token and there was no box to tick
// on /agents to grant it — the storage the documentation told agents to rely on could
// not be granted to one. And the service list is derived from Specs, so a
// capability with no Spec is absent from the catalogue, the sidebar and every
// count, while its tools sit in the surface with nothing to explain them.
//
// The store itself is unchanged: internal/userdb over SQLite, with the same
// owner and public/private model an app gets through mu.db.
//
// The records are not the same records, and the comment this replaces claimed
// they were. An app reads and writes under "apps/<slug>", so each app's data is
// isolated from every other app's; this surface uses "api". Same machinery, same
// guarantees, separate stores — and the only bridge between them is a record
// published with public:true, which either side can read.
package docs

import (
	"context"
	"fmt"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/userdb"
)

// namespace is the store this service reads and writes. Apps get one each under
// "apps/<slug>"; everything reaching in over MCP, REST or this page shares
// "api", scoped per owner.
const namespace = "api"

// Server is the go-micro handler. Its exported methods become the docs_* tools.
type Server struct{}

// caller resolves the authenticated account from call metadata. There is no
// owner field on any request here, deliberately: an argument can be chosen by
// whoever makes the call, context metadata cannot.
func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use your docs")
	}
	return id, nil
}

// ── Create ──────────────────────────────────────────────────────

type CreateRequest struct {
	Collection string         `json:"collection" required:"true" description:"Collection name, e.g. \"notes\" or \"leads\". Made on first write; no schema to declare"`
	Data       map[string]any `json:"data" required:"true" description:"The record's fields as a JSON object"`
	Public     bool           `json:"public,omitempty" description:"Readable by anyone when true. Private by default"`
	ID         string         `json:"id,omitempty" description:"Existing record id to overwrite. Omit to create a new record"`
}

type CreateResponse struct {
	Record *userdb.Record `json:"record" description:"The stored record, including its id"`
}

// Create stores a record, or overwrites one you own when given its id.
// @example {"collection": "leads", "data": {"name": "Sam", "stage": "contacted"}}
func (Server) Create(ctx context.Context, req *CreateRequest, rsp *CreateResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	if req.ID != "" {
		rec, err := userdb.Update(namespace, owner, req.Collection, req.ID, req.Data, req.Public)
		if err != nil {
			return err
		}
		rsp.Record = rec
		return nil
	}
	rec, err := userdb.Create(namespace, owner, req.Collection, req.Data, req.Public)
	if err != nil {
		return err
	}
	rsp.Record = rec
	return nil
}

// ── Get ─────────────────────────────────────────────────────────

type GetRequest struct {
	Collection string `json:"collection" required:"true" description:"Collection name"`
	ID         string `json:"id" required:"true" description:"Record id, as returned by docs_create or docs_list"`
}

type GetResponse struct {
	Record *userdb.Record `json:"record" description:"The record"`
}

// Get returns one record by id. Yours, or one somebody made public.
// @example {"collection": "leads", "id": "8f1c…"}
func (Server) Get(ctx context.Context, req *GetRequest, rsp *GetResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	rec, err := userdb.Get(namespace, owner, req.Collection, req.ID)
	if err != nil {
		return err
	}
	rsp.Record = rec
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct {
	Collection string         `json:"collection" required:"true" description:"Collection name"`
	Scope      string         `json:"scope,omitempty" description:"\"mine\" (default), \"public\", or \"all\" for both"`
	Where      map[string]any `json:"where,omitempty" description:"Filter on data fields, e.g. {\"done\": false, \"priority\": {\"gte\": 2}}"`
	Sort       string         `json:"sort,omitempty" description:"Data field to sort by"`
	Order      string         `json:"order,omitempty" description:"\"asc\" or \"desc\" (default desc)"`
	Limit      int            `json:"limit,omitempty" description:"Max records to return (default 50, max 200)"`
}

type ListResponse struct {
	Records []userdb.Record `json:"records" description:"The matching records, newest first unless sorted otherwise"`
}

// List returns the records in a collection, filtered and sorted.
// @example {"collection": "leads", "where": {"stage": "contacted"}, "limit": 20}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	recs, err := userdb.List(namespace, owner, req.Collection, req.Scope, req.Where, req.Sort, req.Order, req.Limit)
	if err != nil {
		return err
	}
	rsp.Records = recs
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

type DeleteRequest struct {
	Collection string `json:"collection" required:"true" description:"Collection name"`
	ID         string `json:"id" required:"true" description:"Record id to delete. Must be yours"`
}

type DeleteResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Delete removes a record you own.
// @example {"collection": "leads", "id": "8f1c…"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	if err := userdb.Delete(namespace, owner, req.Collection, req.ID); err != nil {
		return err
	}
	rsp.Result = "deleted"
	return nil
}

// LoadService registers docs as a service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("docs", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:    "docs",
	Handler: new(Server),
	// It was "db", labelled "Database", and the two words were the problem. A
	// person reading the catalogue sees Contacts, Events, Mail, News — things
	// they own — and then an implementation detail. What it holds are documents
	// kept in named collections, so that is what it is called, and the old tool
	// names stay as aliases because agents already call them.
	Label:       "Docs",
	Description: "Your own documents — named collections that outlive the conversation",
	Page:        "/docs",
	Icon:        "docs.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		// db_set was db_create's alias, and both are aliases now: nothing that
		// already calls this store should have to be rewritten for a rename it
		// did not ask for.
		"Create": {Doc: "Store a document in one of your collections, or overwrite one you own by passing its id. Private unless you set public",
			Aliases: []string{"db_create", "db_set"}},
		"Get":    {Doc: "Read one document by id — yours, or one somebody published", Aliases: []string{"db_get"}},
		"List":   {Doc: "List documents in a collection, with an optional filter, sort and limit. Use this to find an id before reading or deleting", Aliases: []string{"db_list"}},
		"Delete": {Doc: "Delete a document you own, by id", Destructive: false, Aliases: []string{"db_delete"}},
	},
}

// DeleteAll removes everything docs holds for an owner.
//
// Called when the account is deleted (internal/server/hooks.go). Without it
// the records outlived the account that made them: there was no way to ask
// this store for everything one owner had, so the deletion hooks had nothing
// to call and their own records was simply left behind.
func DeleteAll(owner string) {
	if owner == "" {
		return
	}
	if n, err := userdb.DeleteOwner(namespace, owner); err != nil {
		app.Log("docs", "deleting %s's records: %v", owner, err)
	} else if n > 0 {
		app.Log("docs", "deleted %d records for %s", n, owner)
	}
}
