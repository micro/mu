// Package blob is where user content lives: uploaded files, archived images —
// the things that accumulate and have to survive.
//
// It exists because "the disk" is the wrong answer for those on a hosted
// instance. A droplet's volume is small, is not replicated, and goes with the
// droplet; a user's files should not. Object storage is the right shape, and
// S3's protocol is the one every provider speaks — DigitalOcean Spaces,
// Cloudflare R2, Backblaze B2, MinIO, and S3 itself.
//
// What does *not* belong here is the instance's own working state: news.html,
// markets.html, mail.json, the caches each service rebuilds. Those are derived,
// rewritten constantly, and wanted fast and local. Putting them behind a network
// call would buy nothing and cost latency on every page.
//
// With nothing configured this is the local disk, exactly as before, so a
// self-hoster who wants one binary and a directory keeps having that.
package blob

import (
	"fmt"
	"strings"
	"sync"

	"mu/internal/data"
	"mu/internal/settings"
)

// Store is somewhere bytes live under a key.
type Store interface {
	Put(key string, content []byte, contentType string) error
	Get(key string) ([]byte, error)
	Delete(key string) error
	// Name describes the store for the status page and logs.
	Name() string
}

var (
	mu      sync.Mutex
	current Store
	// configured remembers the settings the current store was built from, so a
	// change at /admin/config takes effect without a restart.
	configured string
)

// Default returns the configured store: S3-compatible when an endpoint and
// bucket are set, otherwise the local disk.
func Default() Store {
	cfg := configFingerprint()

	mu.Lock()
	defer mu.Unlock()
	if current != nil && configured == cfg {
		return current
	}

	s3, err := newS3FromSettings()
	switch {
	case err != nil:
		// A half-configured bucket is a mistake worth seeing, but it must not
		// take the service down: falling back to disk keeps files working while
		// somebody fixes the settings.
		fmt.Printf("blob: object storage is configured but unusable (%v); using local disk\n", err)
		current = Local{}
	case s3 != nil:
		current = s3
	default:
		current = Local{}
	}
	configured = cfg
	return current
}

func configFingerprint() string {
	return strings.Join([]string{
		settings.Get("S3_ENDPOINT"),
		settings.Get("S3_BUCKET"),
		settings.Get("S3_REGION"),
		s3AccessKey(),
		s3SecretKey(),
	}, "|")
}

// Local stores bytes on this machine, under the data directory.
type Local struct{}

func (Local) Put(key string, content []byte, _ string) error {
	return data.SaveFile(key, string(content))
}

func (Local) Get(key string) ([]byte, error) { return data.LoadFile(key) }

func (Local) Delete(key string) error { return data.DeleteFile(key) }

func (Local) Name() string { return "local disk" }

// Put stores bytes in the configured store.
func Put(key string, content []byte, contentType string) error {
	return Default().Put(key, content, contentType)
}

// Get reads bytes back, falling back to the local disk when the configured
// store does not have them.
//
// The fallback is what makes switching to object storage safe on an instance
// that already has files: everything written before the switch is still on
// disk, and would otherwise read as missing. New writes go to the new store, old
// reads keep working, and no migration has to happen before the change is made.
func Get(key string) ([]byte, error) {
	s := Default()
	b, err := s.Get(key)
	if err == nil {
		return b, nil
	}
	if _, isLocal := s.(Local); isLocal {
		return nil, err
	}
	if local, lerr := (Local{}).Get(key); lerr == nil {
		return local, nil
	}
	return nil, err
}

// Delete removes bytes from the configured store, and from the local disk if a
// pre-switch copy is still there.
func Delete(key string) error {
	s := Default()
	err := s.Delete(key)
	if _, isLocal := s.(Local); !isLocal {
		_ = (Local{}).Delete(key)
	}
	return err
}
