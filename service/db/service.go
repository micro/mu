// Package db is per-user storage for services and apps: create, list, get,
// update and delete records in a named collection.
//
// It exists so a service can persist state without importing Mu. The go-micro
// store Mu wires is a single flat keyspace with no owner and no namespace,
// which is the wrong shape for anything personal — reminders, a ledger, notes.
// This wraps internal/userdb, which scopes by (namespace, owner, collection).
//
// The caller is never named in the request. Identity comes from the call
// context, set once at the boundary where a real session exists, so a caller
// cannot reach another user's records by asking for them. See
// internal/service/identity.go.
//
// Headless, like index: a capability with no page of its own.
package db

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/userdb"
	"mu/service/wallet"
)

// ns is the storage namespace for records written through this service, kept
// separate from the namespaces Mu's own packages use (images, apps, …).
const ns = "db"

// Server is the service handler.
type Server struct{}

// Record is one stored item.
type Record struct {
	ID     string                 `json:"id"`
	Owner  string                 `json:"owner"`
	Public bool                   `json:"public"`
	Data   map[string]interface{} `json:"data"`
}

func toRecord(r userdb.Record) Record {
	return Record{ID: r.ID, Owner: r.Owner, Public: r.Public, Data: r.Data}
}

// caller resolves the authenticated account from the call context. There is
// deliberately no account field on any request in this package: an argument can
// be chosen by whoever makes the call, context metadata cannot.
func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use storage")
	}
	return id, nil
}

func collection(name string) (string, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "", fmt.Errorf("collection is required")
	}
	return name, nil
}

// ── Create ──────────────────────────────────────────────────────

type CreateRequest struct {
	Collection string                 `json:"collection" description:"Collection name, e.g. \"notes\""`
	Data       map[string]interface{} `json:"data" description:"The record contents"`
	Public     bool                   `json:"public,omitempty" description:"Share the record publicly (default false)"`
}

type CreateResponse struct {
	Record Record `json:"record" description:"The stored record, including its new id"`
}

// Create stores a new record in a collection owned by the caller.
// @example {"collection": "notes", "data": {"title": "Ideas"}}
func (Server) Create(ctx context.Context, req *CreateRequest, rsp *CreateResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	col, err := collection(req.Collection)
	if err != nil {
		return err
	}
	rec, err := userdb.Create(ns, owner, col, req.Data, req.Public)
	if err != nil {
		return err
	}
	rsp.Record = toRecord(*rec)
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct {
	Collection string                 `json:"collection" description:"Collection name"`
	Scope      string                 `json:"scope,omitempty" description:"\"mine\" (default), \"public\", or \"all\""`
	Where      map[string]interface{} `json:"where,omitempty" description:"Optional field filters"`
	Sort       string                 `json:"sort,omitempty" description:"Field to sort by"`
	Order      string                 `json:"order,omitempty" description:"\"asc\" or \"desc\""`
	Limit      int                    `json:"limit,omitempty" description:"Maximum records to return"`
}

type ListResponse struct {
	Records []Record `json:"records" description:"Matching records, the caller's own unless a wider scope was asked for"`
}

// List returns records from a collection.
// @example {"collection": "notes"}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	col, err := collection(req.Collection)
	if err != nil {
		return err
	}
	scope := req.Scope
	if scope == "" {
		scope = "mine"
	}
	recs, err := userdb.List(ns, who, col, scope, req.Where, req.Sort, req.Order, req.Limit)
	if err != nil {
		return err
	}
	rsp.Records = make([]Record, 0, len(recs))
	for _, r := range recs {
		rsp.Records = append(rsp.Records, toRecord(r))
	}
	return nil
}

// ── Get ─────────────────────────────────────────────────────────

type GetRequest struct {
	Collection string `json:"collection" description:"Collection name"`
	ID         string `json:"id" description:"Record id"`
}

type GetResponse struct {
	Record Record `json:"record" description:"The record, if the caller may see it"`
}

// Get returns one record by id.
// @example {"collection": "notes", "id": "abc123"}
func (Server) Get(ctx context.Context, req *GetRequest, rsp *GetResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	col, err := collection(req.Collection)
	if err != nil {
		return err
	}
	rec, err := userdb.Get(ns, who, col, req.ID)
	if err != nil {
		return err
	}
	rsp.Record = toRecord(*rec)
	return nil
}

// ── Update ──────────────────────────────────────────────────────

type UpdateRequest struct {
	Collection string                 `json:"collection" description:"Collection name"`
	ID         string                 `json:"id" description:"Record id"`
	Data       map[string]interface{} `json:"data" description:"Replacement contents"`
	Public     bool                   `json:"public,omitempty" description:"Share the record publicly"`
}

type UpdateResponse struct {
	Record Record `json:"record" description:"The updated record"`
}

// Update replaces a record the caller owns.
// @example {"collection": "notes", "id": "abc123", "data": {"title": "Better ideas"}}
func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *UpdateResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	col, err := collection(req.Collection)
	if err != nil {
		return err
	}
	rec, err := userdb.Update(ns, who, col, req.ID, req.Data, req.Public)
	if err != nil {
		return err
	}
	rsp.Record = toRecord(*rec)
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

type DeleteRequest struct {
	Collection string `json:"collection" description:"Collection name"`
	ID         string `json:"id" description:"Record id"`
}

type DeleteResponse struct {
	OK bool `json:"ok" description:"True when the record was removed"`
}

// Delete removes a record the caller owns.
// @example {"collection": "notes", "id": "abc123"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	col, err := collection(req.Collection)
	if err != nil {
		return err
	}
	if err := userdb.Delete(ns, who, col, req.ID); err != nil {
		return err
	}
	rsp.OK = true
	return nil
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("db", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "db",
	Handler:     new(Server),
	Description: "Per-user records, for services and apps",
	Label:       "Storage",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		"Create": {Doc: "Store a new record in one of the caller's collections", Cost: wallet.OpDBWrite},
		"Delete": {Doc: "Delete a record the caller owns", Destructive: true},
		"Get":    {Doc: "Read one record by id"},
		"List":   {Doc: "List records from one of the caller's collections"},
		"Update": {Doc: "Replace a record the caller owns", Cost: wallet.OpDBWrite},
	},
}
