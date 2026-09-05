// Package files exposes per-account file storage as a service.
//
// The storage itself lives in internal/files because Files has more than one
// door: HTTP and MCP arrive here, while SFTP arrives through the SSH listener.
// All of them use the same records, quotas and blob.Store underneath.
package files

import (
	"fmt"

	store "mu/internal/files"
)

const (
	MaxBytes      = store.MaxBytes
	MaxOwnerBytes = store.MaxOwnerBytes
)

type File = store.File

func Put(owner, name, contentType, content, encoding string) (*File, error) {
	return store.Put(owner, name, contentType, content, encoding)
}

func Get(caller, id string) (*File, []byte, error) { return store.Get(caller, id) }
func List(owner string) []*File                    { return store.List(owner) }
func Delete(owner, id string) error                { return store.Delete(owner, id) }
func Share(owner, id string, public bool) (*File, error) {
	return store.Share(owner, id, public)
}
func UsedBytes(owner string) int { return store.UsedBytes(owner) }
func DeleteAll(owner string)     { store.DeleteAll(owner) }

// Kept package-local because the page and the existing storage tests describe
// these details from the service surface.
func blobKey(owner, id string) string { return store.BlobKey(owner, id) }

func human(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
