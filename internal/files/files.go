// Package files is per-user file storage: put bytes in, get a URL back, read
// them later.
//
// db stores JSON records and images stores generated pictures. Neither holds a
// file — so an agent that produced a report, a CSV or a transcript had nowhere
// to leave it, and no way to hand a person a link to it. That is the gap this
// fills, and it is the ordinary shape of the request: "keep this and give me a
// link".
//
// Bytes go to internal/blob — the local disk, or S3-compatible object storage
// when the instance is configured for it. The metadata lives in userdb beside
// every other per-user record, so listing, ownership and the private/public
// model are the ones the rest of Mu already uses rather than a second set.
//
// Bytes are never served straight from the bucket. A private file is private
// because Mu checks who is asking, and a public object URL would route around
// that check; the handler streams instead.
//
// The caller is never named in a request. Identity comes from the call context,
// as everywhere else — see internal/service/identity.go.
package files

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/blob"
	"mu/internal/userdb"
)

// ns is the userdb namespace for file metadata, and the prefix for the stored
// bytes on disk.
const ns = "files"

// collection is the single collection file metadata lives in. Files are not
// sorted into collections by the caller: a file has a name, and grouping is
// what tags are for.
const collection = "files"

// MaxBytes caps one file. Big enough for a report, a CSV, a transcript or a
// slide deck; small enough that a runaway agent cannot fill the disk in one
// call. Total storage is bounded per owner by MaxOwnerBytes.
const MaxBytes = 10 << 20 // 10 MiB

// MaxOwnerBytes caps what one account may store in total.
const MaxOwnerBytes = 200 << 20 // 200 MiB

// File is a stored file's metadata. The bytes are not here — they are on disk,
// keyed by Key — because a listing should not carry megabytes.
type File struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Size     int       `json:"size"`
	Public   bool      `json:"public"`
	Owner    string    `json:"owner"`
	URL      string    `json:"url"`
	Checksum string    `json:"checksum"`
	Created  time.Time `json:"created"`
}

// Put stores bytes for an owner and returns the file's metadata.
//
// content is base64 when encoding is "base64", raw text otherwise. An agent
// producing a CSV or a note sends text; one producing a PDF sends base64, and
// should not have to know which the store prefers.
func Put(owner, name, contentType, content, encoding string) (*File, error) {
	if owner == "" {
		return nil, fmt.Errorf("sign in to store files")
	}
	name = cleanName(name)
	if name == "" {
		return nil, fmt.Errorf("a file name is required")
	}

	raw, err := decode(content, encoding)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("the file is empty")
	}
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("file is %s; the limit is %s", human(len(raw)), human(MaxBytes))
	}
	if used := UsedBytes(owner); used+len(raw) > MaxOwnerBytes {
		return nil, fmt.Errorf("storing this would use %s of your %s limit; delete something first",
			human(used+len(raw)), human(MaxOwnerBytes))
	}

	if contentType == "" {
		contentType = typeFor(name)
	}
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])

	rec, err := userdb.Create(ns, owner, collection, map[string]any{
		"name":     name,
		"type":     contentType,
		"size":     len(raw),
		"checksum": checksum,
		"created":  time.Now().UTC().Format(time.RFC3339),
	}, false)
	if err != nil {
		return nil, err
	}

	// Bytes are written after the record so a failed write leaves metadata
	// pointing at nothing rather than bytes nothing points at — the first is
	// visible and fixable, the second is a leak.
	if err := blob.Put(BlobKey(owner, rec.ID), raw, contentType); err != nil {
		_ = userdb.Delete(ns, owner, collection, rec.ID)
		return nil, err
	}

	return toFile(owner, rec.ID, rec.Data, false), nil
}

// Replace writes new contents to an existing file while keeping its identity,
// or creates it when id is empty. SFTP needs filesystem overwrite semantics;
// MCP Put deliberately remains append-only and continues creating records.
func Replace(owner, id, name, contentType string, raw []byte) (*File, error) {
	if id == "" {
		return Put(owner, name, contentType, string(raw), "")
	}
	f, err := meta(owner, id)
	if err != nil || f.Owner != owner {
		return nil, fmt.Errorf("file not found")
	}
	name = cleanName(name)
	if name == "" {
		return nil, fmt.Errorf("a file name is required")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("the file is empty")
	}
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("file is %s; the limit is %s", human(len(raw)), human(MaxBytes))
	}
	if used := UsedBytes(owner) - f.Size + len(raw); used > MaxOwnerBytes {
		return nil, fmt.Errorf("storing this would use %s of your %s limit; delete something first",
			human(used), human(MaxOwnerBytes))
	}
	if contentType == "" {
		contentType = typeFor(name)
	}
	sum := sha256.Sum256(raw)
	data := map[string]any{
		"name": name, "type": contentType, "size": len(raw),
		"checksum": hex.EncodeToString(sum[:]),
		"created":  time.Now().UTC().Format(time.RFC3339),
	}
	if err := blob.Put(BlobKey(owner, id), raw, contentType); err != nil {
		return nil, err
	}
	rec, err := userdb.Update(ns, owner, collection, id, data, f.Public)
	if err != nil {
		return nil, err
	}
	return toFile(owner, id, rec.Data, rec.Public), nil
}

// Rename changes only the name projected by filesystem-shaped doors.
func Rename(owner, id, name string) (*File, error) {
	f, err := meta(owner, id)
	if err != nil || f.Owner != owner {
		return nil, fmt.Errorf("file not found")
	}
	name = cleanName(name)
	if name == "" {
		return nil, fmt.Errorf("a file name is required")
	}
	data := map[string]any{
		"name": name, "type": typeFor(name), "size": f.Size,
		"checksum": f.Checksum, "created": f.Created.UTC().Format(time.RFC3339),
	}
	rec, err := userdb.Update(ns, owner, collection, id, data, f.Public)
	if err != nil {
		return nil, err
	}
	return toFile(owner, id, rec.Data, rec.Public), nil
}

// Get returns a file's metadata and its bytes.
func Get(caller, id string) (*File, []byte, error) {
	f, err := meta(caller, id)
	if err != nil {
		return nil, nil, err
	}
	raw, err := blob.Get(BlobKey(f.Owner, f.ID))
	if err != nil {
		return nil, nil, fmt.Errorf("file %s is missing its contents", id)
	}
	return f, raw, nil
}

// List returns an owner's files, newest first.
func List(owner string) []*File {
	if owner == "" {
		return nil
	}
	recs, err := userdb.List(ns, owner, collection, "mine", nil, "", "", 0)
	if err != nil {
		return nil
	}
	out := make([]*File, 0, len(recs))
	for _, r := range recs {
		out = append(out, toFile(r.Owner, r.ID, r.Data, r.Public))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// Delete removes a file and its bytes.
func Delete(owner, id string) error {
	if _, err := meta(owner, id); err != nil {
		return err
	}
	if err := userdb.Delete(ns, owner, collection, id); err != nil {
		return err
	}
	// A missing blob is not an error here: the record is gone, which is what
	// was asked for, and a file whose bytes already vanished should still be
	// removable rather than permanently stuck.
	_ = blob.Delete(BlobKey(owner, id))
	return nil
}

// Share makes a file readable by anyone with its URL, or private again.
func Share(owner, id string, public bool) (*File, error) {
	f, err := meta(owner, id)
	if err != nil {
		return nil, err
	}
	if f.Owner != owner {
		return nil, fmt.Errorf("file not found")
	}
	rec, err := userdb.Update(ns, owner, collection, id, nil, public)
	if err != nil {
		return nil, err
	}
	return toFile(owner, id, rec.Data, rec.Public), nil
}

// UsedBytes is how much an owner is storing.
func UsedBytes(owner string) int {
	total := 0
	for _, f := range List(owner) {
		total += f.Size
	}
	return total
}

// meta reads a file's record, allowing the owner or — for a public file —
// anyone. Ownership is checked here rather than in each caller so there is one
// place to be wrong.
func meta(caller, id string) (*File, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("a file id is required")
	}
	rec, err := userdb.Get(ns, caller, collection, id)
	if err != nil || rec == nil {
		return nil, fmt.Errorf("file not found")
	}
	return toFile(rec.Owner, rec.ID, rec.Data, rec.Public), nil
}

func toFile(owner, id string, d map[string]any, public bool) *File {
	str := func(k string) string { s, _ := d[k].(string); return s }
	size := 0
	switch v := d["size"].(type) {
	case int:
		size = v
	case float64:
		size = int(v)
	}
	created, _ := time.Parse(time.RFC3339, str("created"))
	return &File{
		ID: id, Name: str("name"), Type: str("type"), Size: size,
		Checksum: str("checksum"), Owner: owner, Public: public,
		Created: created, URL: "/files/" + id,
	}
}

// BlobKey is where a file's bytes live on disk. The owner is in the path so one
// account's files cannot collide with another's even if ids ever repeat.
func BlobKey(owner, id string) string {
	return ns + "/" + safe(owner) + "/" + safe(id)
}

// safe strips anything that could climb out of the data directory. Ids and
// account names are generated, not typed, but this is the one place a caller's
// string becomes a path and it should not depend on that staying true.
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		}
		return '-'
	}, s)
}

// cleanName keeps a file's name to its base, so a caller cannot name a file
// "../../etc/passwd" and have it mean anything.
func cleanName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(filepath.Clean("/" + name))
	if name == "/" || name == "." {
		return ""
	}
	return strings.TrimPrefix(name, "/")
}

func decode(content, encoding string) ([]byte, error) {
	if strings.EqualFold(strings.TrimSpace(encoding), "base64") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(content))
		if err != nil {
			return nil, fmt.Errorf("content is not valid base64: %w", err)
		}
		return raw, nil
	}
	return []byte(content), nil
}

// typeFor guesses a content type from the file's extension, defaulting to
// something a browser will download rather than try to render.
func typeFor(name string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return t
	}
	return "application/octet-stream"
}

func human(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// DeleteAll removes everything files holds for an owner.
//
// Called when the account is deleted (internal/server/hooks.go). Without it
// the records outlived the account that made them: there was no way to ask
// this store for everything one owner had, so the deletion hooks had nothing
// to call and their uploaded files was simply left behind.
func DeleteAll(owner string) {
	if owner == "" {
		return
	}
	if n, err := userdb.DeleteOwner(ns, owner); err != nil {
		app.Log("files", "deleting %s's records: %v", owner, err)
	} else if n > 0 {
		app.Log("files", "deleted %d records for %s", n, owner)
	}
}
