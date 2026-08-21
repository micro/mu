package files

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/service"
)

// Server is the go-micro service handler for files. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools — so "save
// this report" reaches Put and "what have I stored" reaches List.
type Server struct{}

// caller resolves the authenticated account from the call context. There is
// deliberately no owner field on any request here: an argument can be chosen by
// whoever makes the call, context metadata cannot.
func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use file storage")
	}
	return id, nil
}

// ── Put ─────────────────────────────────────────────────────────

type PutRequest struct {
	Name     string `json:"name" required:"true" description:"File name including its extension, e.g. \"report.csv\""`
	Content  string `json:"content" required:"true" description:"The file's contents — plain text, or base64 when encoding is \"base64\""`
	Encoding string `json:"encoding,omitempty" description:"\"base64\" for binary files; omit for text"`
	Type     string `json:"type,omitempty" description:"Optional content type, e.g. \"text/csv\". Guessed from the name when omitted"`
}

type PutResponse struct {
	File   *File  `json:"file" description:"The stored file, including the URL it can be read from"`
	Result string `json:"result" description:"Confirmation, with the file's URL"`
}

// Put stores a file and returns a URL for it. Use it to keep something you
// produced — a report, a CSV, a transcript — and hand back a link to it.
// @example {"name": "report.csv", "content": "date,total\n2026-08-02,41"}
func (Server) Put(ctx context.Context, req *PutRequest, rsp *PutResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	f, err := Put(owner, req.Name, req.Type, req.Content, req.Encoding)
	if err != nil {
		return err
	}
	rsp.File = f
	rsp.Result = fmt.Sprintf("Stored %s (%s) at %s", f.Name, human(f.Size), f.URL)
	return nil
}

// ── Get ─────────────────────────────────────────────────────────

type GetRequest struct {
	ID string `json:"id" required:"true" description:"The file's id, as returned by files_put or files_list"`
}

type GetResponse struct {
	File    *File  `json:"file" description:"The file's metadata"`
	Content string `json:"content" description:"The file's contents; base64 when the file is not text"`
	Binary  bool   `json:"binary" description:"True when content is base64 rather than text"`
}

// Get reads a file back. Text comes back as text; anything else as base64.
// @example {"id": "3d4782a4-ffff-4f89-86bd-8cbf8756bebd"}
func (Server) Get(ctx context.Context, req *GetRequest, rsp *GetResponse) error {
	// A public file is readable by anyone, so this does not require an account —
	// meta() enforces owner-or-public.
	f, raw, err := Get(service.AccountFrom(ctx), req.ID)
	if err != nil {
		return err
	}
	rsp.File = f
	rsp.Content, rsp.Binary = encodeForWire(f.Type, raw)
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct{}

type ListResponse struct {
	Files []*File `json:"files" description:"The caller's files, newest first"`
	Text  string  `json:"text" description:"The same list as model-ready text"`
}

// List returns the caller's stored files, newest first.
// @example {}
func (Server) List(ctx context.Context, _ *ListRequest, rsp *ListResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	rsp.Files = List(owner)
	if len(rsp.Files) == 0 {
		rsp.Text = "No files stored."
		return nil
	}
	var b strings.Builder
	for _, f := range rsp.Files {
		visibility := "private"
		if f.Public {
			visibility = "public"
		}
		fmt.Fprintf(&b, "- %s (%s, %s, %s) — %s [id: %s]\n",
			f.Name, f.Type, human(f.Size), visibility, f.URL, f.ID)
	}
	fmt.Fprintf(&b, "\nUsing %s of %s.", human(UsedBytes(owner)), human(MaxOwnerBytes))
	rsp.Text = b.String()
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

type DeleteRequest struct {
	ID string `json:"id" required:"true" description:"The file's id"`
}

type DeleteResponse struct {
	Result string `json:"result" description:"Confirmation the file was deleted"`
}

// Delete removes a file you own, and its contents.
// @example {"id": "3d4782a4-ffff-4f89-86bd-8cbf8756bebd"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	if err := Delete(owner, req.ID); err != nil {
		return err
	}
	rsp.Result = "Deleted."
	return nil
}

// ── Share ───────────────────────────────────────────────────────

type ShareRequest struct {
	ID     string `json:"id" required:"true" description:"The file's id"`
	Public bool   `json:"public" required:"true" description:"True to let anyone with the URL read it, false to make it private again"`
}

type ShareResponse struct {
	File   *File  `json:"file" description:"The file, with its new visibility"`
	Result string `json:"result" description:"Confirmation, with the URL to share"`
}

// Share makes a file readable by anyone holding its URL, or private again.
// @example {"id": "3d4782a4-ffff-4f89-86bd-8cbf8756bebd", "public": true}
func (Server) Share(ctx context.Context, req *ShareRequest, rsp *ShareResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	f, err := Share(owner, req.ID, req.Public)
	if err != nil {
		return err
	}
	rsp.File = f
	if f.Public {
		rsp.Result = "Anyone with the link can now read " + f.Name + ": " + f.URL
	} else {
		rsp.Result = f.Name + " is private again."
	}
	return nil
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("files", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "files",
	Handler:     new(Server),
	Description: "Per-user file storage: keep a file, get a URL, read it back",
	Page:        "/files",
	Icon:        "files.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		"Put":    {Writes: true, Doc: "Store a file and get a URL for it — a report, a CSV, a transcript"},
		"Get":    {Doc: "Read a stored file back by its id"},
		"List":   {Doc: "List the caller's stored files, newest first"},
		"Delete": {Doc: "Delete a file you own", Destructive: true},
		"Share":  {Writes: true, Doc: "Make a file readable by anyone with its URL, or private again"},
	},
}
