// Package persist commits file-backed state and outbox entries together.
// One process owns an instance's data directory. A write-ahead record is
// recovered before subsequent reads or writes; it is never silently discarded.
package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"mu/internal/dir"
)

var mu sync.Mutex
var poisoned error

const pending = ".pending-commit.json"

func path(key string) (string, error) {
	if key == "" || filepath.IsAbs(key) || filepath.Clean(key) == pending || filepath.Clean(key) == "." {
		return "", fmt.Errorf("invalid persistence key")
	}
	root := dir.Data()
	target := filepath.Join(root, key)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid persistence key")
	}
	return target, nil
}

func atomicFile(target string, b []byte) error {
	if err := durableDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".commit-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), target); err != nil {
		return err
	}
	return syncDir(filepath.Dir(target))
}

// A synced file is not durable if a newly created ancestor directory can still
// disappear after power loss. Persist each new directory's entry in its parent
// before putting transaction files beneath it.
func durableDirectory(name string) error {
	if _, err := os.Stat(name); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(name)
	if parent == name {
		return fmt.Errorf("missing filesystem root")
	}
	if err := durableDirectory(parent); err != nil {
		return err
	}
	if err := os.Mkdir(name, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	return syncDir(parent)
}

func syncDir(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// Batch commits all writes as one recoverable transaction. The outbox is sorted
// after source records by callers' chosen keys. Readers use Read/List/Recover.
func Batch(writes map[string][]byte) error {
	mu.Lock()
	defer mu.Unlock()
	if err := recoverLocked(); err != nil {
		return err
	}
	for key := range writes {
		if _, err := path(key); err != nil {
			return err
		}
	}
	b, err := json.Marshal(writes)
	if err != nil {
		return err
	}
	if err := atomicFile(filepath.Join(dir.Data(), pending), b); err != nil {
		poisoned = fmt.Errorf("uncertain commit needs restart recovery: %w", err)
		return err
	}
	err = recoverLocked()
	if err != nil {
		poisoned = fmt.Errorf("committed write needs restart recovery: %w", err)
	}
	return err
}

func recoverLocked() error {
	if poisoned != nil {
		return poisoned
	}
	name := filepath.Join(dir.Data(), pending)
	b, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var writes map[string][]byte
	if err := json.Unmarshal(b, &writes); err != nil {
		return fmt.Errorf("pending commit needs recovery: %w", err)
	}
	keys := make([]string, 0, len(writes))
	for key := range writes {
		if _, err := path(key); err != nil {
			return err
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		target, _ := path(key)
		if err := atomicFile(target, writes[key]); err != nil {
			return err
		}
	}
	if err := os.Remove(name); err != nil {
		return err
	}
	return syncDir(dir.Data())
}

func Recover() error {
	mu.Lock()
	defer mu.Unlock()
	return recoverLocked()
}

func Write(key string, b []byte) error {
	mu.Lock()
	defer mu.Unlock()
	if err := recoverLocked(); err != nil {
		return err
	}
	target, err := path(key)
	if err != nil {
		return err
	}
	return atomicFile(target, b)
}

func Read(key string) ([]byte, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := recoverLocked(); err != nil {
		return nil, err
	}
	target, err := path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(target)
}

func List(key string) ([]string, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := recoverLocked(); err != nil {
		return nil, err
	}
	target, err := path(key)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(target)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// WritePath preserves existing callers of atomic file writes within Data.
func WritePath(target string, b []byte) error {
	key, err := filepath.Rel(dir.Data(), target)
	if err != nil {
		return err
	}
	return Write(key, b)
}
