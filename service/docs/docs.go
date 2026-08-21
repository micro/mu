// Package docs is the caller's own documents: a title, a body, and nothing else
// to learn.
//
// It was a database with a documents label. The tool took a collection name and
// a bag of JSON — docs_create(collection, data) is INSERT INTO — and the page
// asked you to type that JSON by hand, with an error branch whose comment
// called invalid JSON "the most common mistake by a distance". When a page's
// commonest error is a syntax error in its input format, the input format is the
// bug.
//
// The rename from db to docs fixed the label and left the room alone. This is
// the room.
//
// A document is what a person means by one: something you write, re-read, and
// come back to. Underneath it is still a userdb record — a record with a title
// and a body is a document, and none of the ownership, privacy or storage model
// had to change. What changed is that the caller no longer has to know that.
//
// Not notes, deliberately. A note is short, ambient, and read back into every
// conversation; you accumulate notes rather than open them. A document is
// written on purpose and opened again later. Same storage, different lifecycle,
// and merging them would cost each one the thing that makes it useful.
//
// The general record store still exists and is still what apps persist through
// — internal/userdb, reached by apps as mu.db. It has no page and no tools,
// because "a database" is not a kind of thing a person makes. It is how all the
// kinds are stored.
package docs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/userdb"
)

const (
	// namespace stays "api" — the same one the record store has always used, so
	// nothing on disk moves.
	namespace = "api"

	// collection is where documents live within it. One collection, because a
	// document does not belong to a table; asking a person to name one before
	// they can write anything was the whole problem.
	collection = "documents"

	// maxTitle and maxBody bound one document. Generous enough for anything
	// anybody types, small enough that one document cannot fill the disk.
	maxTitle = 200
	maxBody  = 200000
)

// Doc is one document.
type Doc struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
	Public  bool      `json:"public"`
	Updated time.Time `json:"updated"`
}

// Server is the go-micro service handler for documents.
type Server struct{}

// caller is the account this call belongs to. Identity comes from the context
// and never from a request field.
func caller(ctx context.Context) (string, error) {
	who := service.AccountFrom(ctx)
	if who == "" {
		return "", fmt.Errorf("sign in to use documents")
	}
	return who, nil
}

// ── Write ───────────────────────────────────────────────────────

// WriteRequest creates a document, or replaces one by id.
type WriteRequest struct {
	Title   string `json:"title" required:"true" description:"The document's title"`
	Content string `json:"content" required:"true" description:"The document's body, as markdown"`
	ID      string `json:"id,omitempty" description:"Existing document id to replace. Omit to create a new one"`
	Public  bool   `json:"public,omitempty" description:"Readable by anyone when true. Private by default"`
}

// WriteResponse confirms the write.
type WriteResponse struct {
	Text string `json:"text" description:"Confirmation, with the document's id"`
	Doc  *Doc   `json:"doc" description:"The stored document"`
}

// Write stores a document — a title and a body. Pass an id to replace one.
// @example {"title": "Notes on the Q3 plan", "content": "# Q3\n\nThe short version is..."}
func (Server) Write(ctx context.Context, req *WriteRequest, rsp *WriteResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	doc, err := Save(who, req.ID, req.Title, req.Content, req.Public)
	if err != nil {
		return err
	}
	rsp.Doc = doc
	rsp.Text = fmt.Sprintf("Saved %q [id: %s]", doc.Title, doc.ID)
	return nil
}

// ── Read ────────────────────────────────────────────────────────

// ReadRequest selects one document.
type ReadRequest struct {
	ID    string `json:"id,omitempty" description:"Document id, from docs_list"`
	Title string `json:"title,omitempty" description:"Exact title, if you do not have the id"`
}

// ReadResponse is one document in full.
type ReadResponse struct {
	Text string `json:"text" description:"The document's title and body"`
	Doc  *Doc   `json:"doc" description:"The document"`
}

// Read returns one document in full, by id or by exact title.
// @example {"title": "Notes on the Q3 plan"}
func (Server) Read(ctx context.Context, req *ReadRequest, rsp *ReadResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	doc := Get(who, req.ID)
	if doc == nil && strings.TrimSpace(req.Title) != "" {
		doc = ByTitle(who, req.Title)
	}
	if doc == nil {
		rsp.Text = "No such document. Use docs_list to see what is there."
		return nil
	}
	rsp.Doc = doc
	rsp.Text = doc.Title + "\n\n" + doc.Content
	return nil
}

// ── List ────────────────────────────────────────────────────────

// ListRequest optionally narrows the list.
type ListRequest struct {
	Query string `json:"query,omitempty" description:"Optional text to match against titles and bodies"`
	Limit int    `json:"limit,omitempty" description:"Maximum documents to return (default 50)"`
}

// ListResponse is the caller's documents.
type ListResponse struct {
	Text string `json:"text" description:"The caller's documents: title, id and when each was last changed"`
	Docs []*Doc `json:"docs" description:"The same documents as data"`
}

// List returns the caller's documents, most recently changed first.
// @example {}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	docs := All(who, req.Query, req.Limit)
	rsp.Docs = docs
	rsp.Text = Render(docs, req.Query)
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

// DeleteRequest selects the document to remove.
type DeleteRequest struct {
	ID string `json:"id" required:"true" description:"Document id, from docs_list"`
}

// DeleteResponse confirms the deletion.
type DeleteResponse struct {
	Text string `json:"text" description:"Confirmation"`
}

// Delete removes one of the caller's documents.
// @example {"id": "8f1c…"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if err := Remove(who, req.ID); err != nil {
		return err
	}
	rsp.Text = "Deleted."
	return nil
}

// ── The store ───────────────────────────────────────────────────

// Save writes a document, replacing one when id is given.
func Save(owner, id, title, content string, public bool) (*Doc, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("a document needs a title")
	}
	if len(title) > maxTitle {
		return nil, fmt.Errorf("that title is too long — %d characters is the limit", maxTitle)
	}
	if len(content) > maxBody {
		return nil, fmt.Errorf("that document is too long — %d characters is the limit", maxBody)
	}

	data := map[string]any{
		"title":   title,
		"content": content,
		"updated": time.Now().Format(time.RFC3339),
	}
	// Replace when given an id, create otherwise. Replacing is a write to the
	// same record rather than a delete and a new one, so the id a caller already
	// holds keeps working.
	if strings.TrimSpace(id) != "" {
		rec, err := userdb.Update(namespace, owner, collection, id, data, public)
		if err != nil {
			return nil, err
		}
		return toDoc(rec), nil
	}
	rec, err := userdb.Create(namespace, owner, collection, data, public)
	if err != nil {
		return nil, err
	}
	return toDoc(rec), nil
}

// Get returns one document by id, or nil.
func Get(owner, id string) *Doc {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	rec, err := userdb.Get(namespace, owner, collection, id)
	if err != nil || rec == nil {
		return nil
	}
	return toDoc(rec)
}

// ByTitle returns the caller's document with this exact title, or nil. Exact,
// because guessing which document somebody meant is how you overwrite the wrong
// one.
func ByTitle(owner, title string) *Doc {
	title = strings.TrimSpace(title)
	for _, d := range All(owner, "", 0) {
		if strings.EqualFold(d.Title, title) {
			return d
		}
	}
	return nil
}

// All returns the caller's documents, most recently changed first, optionally
// matching a query against title and body.
func All(owner, query string, limit int) []*Doc {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	recs, err := userdb.List(namespace, owner, collection, "mine", nil, "", "", 500)
	if err != nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []*Doc
	for i := range recs {
		d := toDoc(&recs[i])
		if q != "" && !strings.Contains(strings.ToLower(d.Title), q) &&
			!strings.Contains(strings.ToLower(d.Content), q) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Remove deletes one of the caller's documents.
func Remove(owner, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("which document? pass the id from docs_list")
	}
	return userdb.Delete(namespace, owner, collection, id)
}

// Render writes documents the way a model should read them: enough to choose
// one, not the whole body of every one.
func Render(docs []*Doc, query string) string {
	if len(docs) == 0 {
		if strings.TrimSpace(query) != "" {
			return fmt.Sprintf("No documents matching %q.", query)
		}
		return "No documents yet."
	}
	var b strings.Builder
	for _, d := range docs {
		fmt.Fprintf(&b, "- %s", d.Title)
		if snip := snippet(d.Content); snip != "" {
			fmt.Fprintf(&b, " — %s", snip)
		}
		fmt.Fprintf(&b, " [id: %s]\n", d.ID)
	}
	return strings.TrimSpace(b.String())
}

// snippet is the first line of a body, short enough to sit on a list row.
func snippet(content string) string {
	s := strings.TrimSpace(content)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(strings.TrimLeft(s, "# "))
	if len(s) > 100 {
		s = s[:97] + "..."
	}
	return s
}

func toDoc(r *userdb.Record) *Doc {
	if r == nil {
		return nil
	}
	title, _ := r.Data["title"].(string)
	content, _ := r.Data["content"].(string)
	updated, _ := r.Data["updated"].(string)
	t, _ := time.Parse(time.RFC3339, updated)
	return &Doc{ID: r.ID, Title: title, Content: content, Public: r.Public, Updated: t}
}

// LoadService registers the documents service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("docs", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:    "docs",
	Handler: new(Server),
	// It was "db", labelled "Database", and renaming it to Docs fixed the word
	// while leaving a database behind it: the tool took a collection and a bag
	// of JSON, and the page asked a person to type that JSON. A service is named
	// for a kind of thing somebody makes — a document, a note, a spreadsheet —
	// never for how it is stored.
	Label:       "Docs",
	Description: "Your own documents — write, keep and come back to them",
	Page:        "/docs",
	Icon:        "docs.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		"Write":  {Writes: true, Doc: "Write a document — a title and a markdown body. Pass an id to replace one you already have. Private unless you set public. For something long enough to re-read; for a short thing to remember, use notes"},
		"Read":   {Doc: "Read one of your documents in full, by id or by exact title"},
		"List":   {Doc: "List your documents, most recently changed first, with an optional search over titles and bodies. Use this to find an id"},
		"Delete": {Doc: "Delete one of your documents, by id", Destructive: true},
	},
}

// DeleteAll removes every document an owner has.
//
// Called when the account is deleted (internal/server/hooks.go). Without it the
// records outlive the account that made them.
func DeleteAll(owner string) {
	if owner == "" {
		return
	}
	if n, err := userdb.DeleteOwner(namespace, owner); err != nil {
		app.Log("docs", "deleting %s's documents: %v", owner, err)
	} else if n > 0 {
		app.Log("docs", "deleted %d documents for %s", n, owner)
	}
}
